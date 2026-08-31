//go:build windows

package api

import (
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	_ "modernc.org/sqlite"
)

// BrowserCookieExtractor 从 Chrome/Edge 浏览器 Cookie 数据库提取火山控制台 Cookie。
// 参考 CodexBar 对 Claude/Cursor 的浏览器 Cookie 提取方式。
type BrowserCookieExtractor struct {
	logger *slog.Logger
}

// NewBrowserCookieExtractor 创建浏览器 Cookie 提取器。
func NewBrowserCookieExtractor(logger *slog.Logger) *BrowserCookieExtractor {
	if logger == nil {
		logger = slog.Default()
	}
	return &BrowserCookieExtractor{logger: logger}
}

// Extract 提取 console.volcengine.com 的 Cookie 字符串。
// 依次尝试 Edge 与 Chrome；返回第一个成功提取的 Cookie。
func (e *BrowserCookieExtractor) Extract() (string, error) {
	for _, browser := range []struct {
		name       string
		cookiePath string
		localState string
	}{
		{"Edge", edgeCookieDB(), edgeLocalState()},
		{"Chrome", chromeCookieDB(), chromeLocalState()},
	} {
		cookie, err := e.extractFromBrowser(browser.name, browser.cookiePath, browser.localState)
		if err != nil {
			e.logger.Debug("browser cookie extraction failed",
				"browser", browser.name, "error", err)
			continue
		}
		if cookie != "" {
			e.logger.Info("browser cookie extracted", "browser", browser.name)
			return cookie, nil
		}
	}
	return "", fmt.Errorf("未找到可用的浏览器 Cookie（Edge/Chrome 均失败）")
}

// extractFromBrowser 从单个浏览器提取 Cookie。
func (e *BrowserCookieExtractor) extractFromBrowser(browser, dbPath, localStatePath string) (string, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return "", fmt.Errorf("Cookie 数据库不存在: %w", err)
	}

	// 复制 DB + WAL 到临时目录，避免浏览器文件锁
	tmpDir, err := os.MkdirTemp("", "ark-cookie-*")
	if err != nil {
		return "", fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpDB := filepath.Join(tmpDir, "Cookies")
	if err := copyFile(dbPath, tmpDB); err != nil {
		return "", fmt.Errorf("复制 Cookie 数据库失败: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(dbPath + suffix); err == nil {
			_ = copyFile(dbPath+suffix, tmpDB+suffix)
		}
	}

	// 读取加密密钥（v20 需要）
	key, err := loadEncryptionKey(localStatePath)
	if err != nil {
		e.logger.Debug("load encryption key failed, will try v10 only", "error", err)
	}

	// 查询并解密 cookies
	cookies, err := readCookies(tmpDB, "console.volcengine.com")
	if err != nil {
		return "", fmt.Errorf("读取 Cookie 数据库失败: %w", err)
	}

	var parts []string
	for _, c := range cookies {
		val, err := decryptCookieValue(c.encryptedValue, key)
		if err != nil {
			e.logger.Debug("cookie decrypt failed", "name", c.name, "error", err)
			continue
		}
		parts = append(parts, c.name+"="+val)
	}

	if len(parts) == 0 {
		return "", fmt.Errorf("未找到可解密的 console.volcengine.com Cookie")
	}
	return strings.Join(parts, "; "), nil
}

// cookieRecord 是 Cookie 数据库中的一行。
type cookieRecord struct {
	name           string
	encryptedValue []byte
}

// readCookies 从 SQLite Cookie 数据库读取指定主机的 Cookie。
func readCookies(dbPath, host string) ([]cookieRecord, error) {
	dsn := "file:" + filepath.ToSlash(dbPath) + "?mode=ro&_pragma=query_only(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(
		`SELECT name, encrypted_value FROM cookies WHERE host_key LIKE ?`,
		"%"+host+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []cookieRecord
	for rows.Next() {
		var name string
		var enc []byte
		if err := rows.Scan(&name, &enc); err != nil {
			continue
		}
		result = append(result, cookieRecord{name: name, encryptedValue: enc})
	}
	return result, rows.Err()
}

// loadEncryptionKey 从 Local State 文件读取并解密 AES 密钥（v20 加密格式）。
func loadEncryptionKey(localStatePath string) ([]byte, error) {
	data, err := os.ReadFile(localStatePath)
	if err != nil {
		return nil, err
	}
	var state struct {
		OSCrypt struct {
			EncryptedKey string `json:"encrypted_key"`
		} `json:"os_crypt"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.OSCrypt.EncryptedKey == "" {
		return nil, fmt.Errorf("Local State 无 encrypted_key")
	}
	encKey, err := base64.StdEncoding.DecodeString(state.OSCrypt.EncryptedKey)
	if err != nil {
		return nil, err
	}
	// 去掉 "DPAPI" 前缀
	if !strings.HasPrefix(string(encKey), "DPAPI") {
		return nil, fmt.Errorf("encrypted_key 非 DPAPI 格式")
	}
	return decryptDPAPI(encKey[5:])
}

// decryptCookieValue 解密 Cookie 值，支持 v10（DPAPI）与 v20（AES-GCM）格式。
func decryptCookieValue(encrypted []byte, key []byte) (string, error) {
	if len(encrypted) < 3 {
		return "", fmt.Errorf("加密值过短")
	}
	prefix := string(encrypted[:3])
	switch prefix {
	case "v10":
		plain, err := decryptDPAPI(encrypted[3:])
		if err != nil {
			return "", err
		}
		return string(plain), nil
	case "v20":
		if len(key) == 0 {
			return "", fmt.Errorf("v20 需要 AES 密钥")
		}
		plain, err := decryptAESGCM(key, encrypted[3:])
		if err != nil {
			return "", err
		}
		return string(plain), nil
	default:
		// 未加密的 Cookie（部分浏览器/场景）
		return string(encrypted), nil
	}
}

// decryptAESGCM 使用 AES-GCM 解密（v20 格式：前 12 字节为 nonce，其余为密文+tag）。
func decryptAESGCM(key, data []byte) ([]byte, error) {
	if len(data) < 12+16 {
		return nil, fmt.Errorf("v20 数据过短")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := data[:12]
	ciphertext := data[12:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// decryptDPAPI 使用 Windows DPAPI 解密数据。
func decryptDPAPI(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("DPAPI 数据为空")
	}
	in := windows.DataBlob{Size: uint32(len(data)), Data: &data[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, 0, &out); err != nil {
		return nil, fmt.Errorf("DPAPI 解密失败: %w", err)
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(out.Data))))
	return unsafe.Slice(out.Data, out.Size), nil
}

// copyFile 复制文件。
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

// edgeCookieDB / edgeLocalState / chromeCookieDB / chromeLocalState 返回浏览器数据路径。
func edgeCookieDB() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "User Data", "Default", "Network", "Cookies")
}
func edgeLocalState() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "User Data", "Local State")
}
func chromeCookieDB() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data", "Default", "Network", "Cookies")
}
func chromeLocalState() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data", "Local State")
}