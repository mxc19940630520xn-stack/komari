package geoip // 与 geoip.go 保持相同的包名，表示它们是同一个包的组成部分

import (
	"fmt"
	logger "github.com/komari-monitor/komari/utils/log"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath" // 新增导入，用于处理文件路径
	"sync"
	"time"

	"github.com/komari-monitor/komari/database/auditlog"
	"github.com/oschwald/maxminddb-golang"
)

// GeoIpUrl 是 MaxMind 数据库的下载地址。
var GeoIpUrl = "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/GeoLite2-Country.mmdb"

// GeoIpFilePath 是本地存储 MaxMind 数据库的路径。
var GeoIpFilePath = "./data/GeoLite2-Country.mmdb"

// GeoIpRecord 结构体定义了 MaxMind 数据库查询结果的原始结构。
// 它是 MaxMind 库特有的，用于从 .mmdb 文件中解析数据。
type GeoIpRecord struct {
	Country struct {
		ISOCode string            `maxminddb:"iso_code"`
		Names   map[string]string `maxminddb:"names"`
	} `maxminddb:"country"`
}

// MaxMindGeoIPService 是 GeoIPService 接口的一个具体实现，
// 它使用 MaxMind 数据库作为后端。
type MaxMindGeoIPService struct {
	// maxMindDBReader 内部持有的 MaxMind 数据库读取器实例。
	maxMindDBReader *maxminddb.Reader
	// dbFilePath 是 MaxMind 数据库文件的路径。
	dbFilePath string
	// mu 用于保护对 maxMindDBReader 的并发访问，确保线程安全。
	mu sync.RWMutex
	// Serialize updates independently so lookups can keep using the current DB.
	updateMu sync.Mutex
}

// Name 返回服务的名称。
func (s *MaxMindGeoIPService) Name() string {
	return "MaxMind"
}

// NewMaxMindGeoIPService 创建并返回一个 MaxMindGeoIPService 实例。
// 它负责初始化服务，包括尝试加载或下载数据库。
func NewMaxMindGeoIPService() (*MaxMindGeoIPService, error) {
	dbFilePath := GeoIpFilePath
	service := &MaxMindGeoIPService{
		dbFilePath: dbFilePath,
	}

	// 确保数据目录存在
	if err := os.MkdirAll(filepath.Dir(dbFilePath), os.ModePerm); err != nil {
		auditlog.Log("", "", "Failed to create data directory for MaxMind database: "+err.Error(), "error")
		return nil, fmt.Errorf("failed to create data directory for MaxMind database: %w", err)
	}

	// 检查数据库文件是否存在，如果不存在则尝试下载
	if _, err := os.Stat(dbFilePath); os.IsNotExist(err) {
		if err := service.UpdateDatabase(); err != nil {
			auditlog.Log("", "", "Failed to download initial MaxMind database: "+err.Error(), "error")
			return nil, fmt.Errorf("failed to download initial MaxMind database: %w", err)
		}
	}

	// 初始化或重新加载 MaxMind 数据库。
	if err := service.initialize(); err != nil {
		auditlog.Log("", "", "Failed to initialize MaxMind database: "+err.Error(), "error")
		return nil, fmt.Errorf("failed to initialize MaxMind database: %w", err)
	}
	return service, nil
}

// initialize 初始化或重新加载 MaxMind 数据库。
// 这是一个内部方法，供 NewMaxMindGeoIPService 和 UpdateDatabase 调用。
// 它会关闭现有连接（如果存在）并重新打开数据库文件。
func (s *MaxMindGeoIPService) initialize() error {
	data, err := os.ReadFile(s.dbFilePath)
	if err != nil {
		return fmt.Errorf("read MaxMind database: %w", err)
	}
	// Keep readers in memory: a mapped file cannot be replaced on Windows.
	reader, err := maxminddb.FromBytes(data)
	if err != nil {
		return fmt.Errorf("open MaxMind database: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.maxMindDBReader != nil {
		s.maxMindDBReader.Close()
	}
	s.maxMindDBReader = reader
	return nil
}

// GetGeoInfo 根据 IP 地址获取 MaxMind 的地理位置信息。
// 它查询 MaxMind 数据库并将其特有的 GeoIpRecord 转换为通用的 GeoInfo 结构体。
func (s *MaxMindGeoIPService) GetGeoInfo(ip net.IP) (*GeoInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.maxMindDBReader == nil {
		return nil, fmt.Errorf("MaxMind database is not initialized or failed to open")
	}
	if ip == nil {
		return nil, fmt.Errorf("IP address cannot be nil")
	}

	var record GeoIpRecord // 使用原始的 GeoIpRecord 结构体来接收查询结果
	err := s.maxMindDBReader.Lookup(ip, &record)
	if err != nil {
		// 返回错误，但避免直接返回 maxminddb 库的内部错误，提供更友好的信息
		return nil, fmt.Errorf("error looking up IP %s in MaxMind database: %w", ip.String(), err)
	}

	// 将 MaxMind 的特定结构体转换为通用的 GeoInfo 结构体
	geoInfo := &GeoInfo{
		ISOCode: record.Country.ISOCode,
		// 尝试获取英文国家名称，如果不存在则使用 ISO 代码作为备用
		Name: record.Country.Names["en"],
	}
	if geoInfo.Name == "" && geoInfo.ISOCode != "" {
		geoInfo.Name = geoInfo.ISOCode // 如果没有英文名称，回退到 ISO 代码
	}
	return geoInfo, nil
}

// UpdateDatabase 实现了 GeoIPService 接口的 UpdateDatabase 方法。
// 它会下载最新的 GeoLite2-Country.mmdb 文件并重新加载数据库。
func (s *MaxMindGeoIPService) UpdateDatabase() error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(GeoIpUrl)
	if err != nil {
		return fmt.Errorf("failed to initiate MaxMind database download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download MaxMind database: HTTP status %s", resp.Status)
	}

	const maxDatabaseSize = 64 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDatabaseSize+1))
	if err != nil {
		return fmt.Errorf("read MaxMind database download: %w", err)
	}
	if len(data) > maxDatabaseSize {
		return fmt.Errorf("MaxMind database exceeds %d bytes", maxDatabaseSize)
	}
	reader, err := maxminddb.FromBytes(data)
	if err != nil {
		return fmt.Errorf("invalid MaxMind database download: %w", err)
	}
	defer func() {
		if reader != nil {
			_ = reader.Close()
		}
	}()
	if err := reader.Verify(); err != nil {
		return fmt.Errorf("verify MaxMind database download: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.dbFilePath), os.ModePerm); err != nil {
		return fmt.Errorf("failed to create data directory for MaxMind database update: %w", err)
	}

	out, err := os.CreateTemp(filepath.Dir(s.dbFilePath), ".geoip-*.mmdb")
	if err != nil {
		return fmt.Errorf("failed to create MaxMind database file at %s: %w", s.dbFilePath, err)
	}
	defer func() {
		_ = out.Close()
		_ = os.Remove(out.Name())
	}()
	if _, err := out.Write(data); err != nil {
		return fmt.Errorf("failed to write MaxMind database file: %w", err)
	}
	if err := out.Sync(); err != nil {
		return fmt.Errorf("sync MaxMind database: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close MaxMind database: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Rename(out.Name(), s.dbFilePath); err != nil {
		return fmt.Errorf("replace MaxMind database: %w", err)
	}
	if s.maxMindDBReader != nil {
		_ = s.maxMindDBReader.Close()
	}
	s.maxMindDBReader = reader
	reader = nil
	return nil
}

// Close 实现了 GeoIPService 接口的 Close 方法。
// 它关闭 MaxMind 数据库读取器，释放文件句柄和其他资源。
func (s *MaxMindGeoIPService) Close() error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.maxMindDBReader != nil {
		err := s.maxMindDBReader.Close()
		s.maxMindDBReader = nil // 清空读取器实例
		if err != nil {
			return fmt.Errorf("error closing MaxMind database: %w", err)
		}
	}
	logger.InfoArgs("geoip", "MaxMind GeoIP service closed.")
	return nil
}
