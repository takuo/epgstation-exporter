package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/takuo/epgstation-exporter/pkg/epgstation"
)

const namespace = "epgstation"

const (
	defaultChannelCacheTTL = 60 * time.Minute
	failedChannelCacheTTL  = 1 * time.Minute
)

// Collector はEPGStation APIからメトリクスを収集するPrometheusコレクター
type Collector struct {
	client              *epgstation.ClientWithResponses
	httpClient          *http.Client
	apiURL              string
	enableStorage       bool
	enableStreams       bool
	enableRecordingInfo bool

	// チャンネル名キャッシュ
	channelMu          sync.RWMutex
	channelNames       map[int]string
	nextChannelRefresh time.Time

	// メトリクス定義
	up                *prometheus.Desc
	info              *prometheus.Desc
	reservesTotal     *prometheus.Desc
	recordingTotal    *prometheus.Desc
	storageAvailable  *prometheus.Desc
	storageUsed       *prometheus.Desc
	storageTotal      *prometheus.Desc
	encodeRunning     *prometheus.Desc
	encodeWaiting     *prometheus.Desc
	streamsTotal      *prometheus.Desc
	rulesTotal        *prometheus.Desc
	ruleReservesTotal *prometheus.Desc
	recordedTotal     *prometheus.Desc
	recordingInfo     *prometheus.Desc
}

// New は新しいCollectorを作成する
func New(apiURL string, enableStorage bool, enableStreams bool, enableRecordingInfo bool) (*Collector, error) {
	client, err := epgstation.NewClientWithResponses(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create epgstation client: %w", err)
	}
	return NewWithClient(client, apiURL, enableStorage, enableStreams, enableRecordingInfo), nil
}

// NewWithClient は既存のClientWithResponsesを使ってCollectorを作成する（テスト用）
func NewWithClient(client *epgstation.ClientWithResponses, apiURL string, enableStorage bool, enableStreams bool, enableRecordingInfo bool) *Collector {
	return &Collector{
		client:              client,
		httpClient:          &http.Client{},
		apiURL:              apiURL,
		enableStorage:       enableStorage,
		enableStreams:       enableStreams,
		enableRecordingInfo: enableRecordingInfo,

		up: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "up"),
			"Whether EPGStation is running (1: running, 0: down)",
			nil, nil,
		),
		info: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "", "info"),
			"EPGStation version information",
			[]string{"version", "url"}, nil,
		),
		reservesTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "reserves", "total"),
			"Total number of reserves",
			[]string{"type"}, nil,
		),
		recordingTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "recording", "total"),
			"Number of programs currently recording",
			nil, nil,
		),
		storageAvailable: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "storage_available", "bytes"),
			"Available storage space in bytes",
			[]string{"name"}, nil,
		),
		storageUsed: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "storage_used", "bytes"),
			"Used storage space in bytes",
			[]string{"name"}, nil,
		),
		storageTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "storage_total", "bytes"),
			"Total storage capacity in bytes",
			[]string{"name"}, nil,
		),
		encodeRunning: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "encode_running", "total"),
			"Number of running encode jobs",
			nil, nil,
		),
		encodeWaiting: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "encode_waiting", "total"),
			"Number of waiting encode jobs",
			nil, nil,
		),
		streamsTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "streams", "total"),
			"Total number of streams",
			[]string{"type"}, nil,
		),
		rulesTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "rules", "total"),
			"Total number of rules",
			[]string{"enabled"}, nil,
		),
		ruleReservesTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "rule_reserves", "total"),
			"Number of reserves per rule",
			[]string{"id", "name"}, nil,
		),
		recordedTotal: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "recorded", "total"),
			"Total number of recorded programs in the library",
			nil, nil,
		),
		recordingInfo: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "recording", "info"),
			"Information about currently recording programs",
			[]string{"id", "title", "channel_id", "channel_name", "start_at", "end_at", "genre"}, nil,
		),
	}
}

// Describe はPrometheus Collectorインターフェースを実装する
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.up
	ch <- c.info
	ch <- c.reservesTotal
	ch <- c.recordingTotal
	if c.enableStorage {
		ch <- c.storageAvailable
		ch <- c.storageUsed
		ch <- c.storageTotal
	}
	ch <- c.encodeRunning
	ch <- c.encodeWaiting
	if c.enableStreams {
		ch <- c.streamsTotal
	}
	ch <- c.rulesTotal
	ch <- c.ruleReservesTotal
	ch <- c.recordedTotal
	if c.enableRecordingInfo {
		ch <- c.recordingInfo
	}
}

// Collect はPrometheus Collectorインターフェースを実装する
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()

	if err := c.collectVersion(ctx, ch); err != nil {
		slog.Error("failed to collect version metrics", "err", err)
		ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 0)
		return
	}

	if err := c.collectReserves(ctx, ch); err != nil {
		slog.Error("failed to collect reserves metrics", "err", err)
	}

	if err := c.collectRecording(ctx, ch); err != nil {
		slog.Error("failed to collect recording metrics", "err", err)
	}

	if c.enableStorage {
		if err := c.collectStorages(ctx, ch); err != nil {
			slog.Error("failed to collect storage metrics", "err", err)
		}
	}

	if err := c.collectEncode(ctx, ch); err != nil {
		slog.Error("failed to collect encode metrics", "err", err)
	}

	if c.enableStreams {
		if err := c.collectStreams(ctx, ch); err != nil {
			slog.Error("failed to collect streams metrics", "err", err)
		}
	}

	if err := c.collectRules(ctx, ch); err != nil {
		slog.Error("failed to collect rules metrics", "err", err)
	}

	if err := c.collectRecorded(ctx, ch); err != nil {
		slog.Error("failed to collect recorded metrics", "err", err)
	}
}

// getChannelNames はチャンネルID->名前のマップを返す。TTL内はキャッシュを使用する。
func (c *Collector) getChannelNames(ctx context.Context) map[int]string {
	c.channelMu.RLock()
	if c.channelNames != nil && time.Now().Before(c.nextChannelRefresh) {
		names := c.channelNames
		c.channelMu.RUnlock()
		return names
	}
	c.channelMu.RUnlock()

	c.channelMu.Lock()
	defer c.channelMu.Unlock()
	// ダブルチェック
	if c.channelNames != nil && time.Now().Before(c.nextChannelRefresh) {
		return c.channelNames
	}

	resp, err := c.client.GetChannelsWithResponse(ctx)
	if err != nil || resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		if err != nil {
			slog.Warn("failed to fetch channels", "err", err)
		} else {
			slog.Warn("unexpected status from channels API", "status", resp.Status())
		}
		if c.channelNames == nil {
			c.channelNames = make(map[int]string)
		}
		c.nextChannelRefresh = time.Now().Add(failedChannelCacheTTL)
		return c.channelNames
	}
	names := make(map[int]string, len(*resp.JSON200))
	for _, ch := range *resp.JSON200 {
		names[ch.Id] = ch.Name
	}
	c.channelNames = names
	c.nextChannelRefresh = time.Now().Add(defaultChannelCacheTTL)
	slog.Debug("channel cache refreshed", "count", len(names))
	return names
}

func (c *Collector) collectVersion(ctx context.Context, ch chan<- prometheus.Metric) error {
	resp, err := c.client.GetVersionWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return fmt.Errorf("unexpected status: %s", resp.Status())
	}

	ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, 1)
	baseURL := strings.TrimSuffix(strings.TrimRight(c.apiURL, "/"), "/api")
	ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1, resp.JSON200.Version, baseURL)
	return nil
}

func (c *Collector) collectReserves(ctx context.Context, ch chan<- prometheus.Metric) error {
	resp, err := c.client.GetReservesCntsWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return fmt.Errorf("unexpected status: %s", resp.Status())
	}

	cnts := resp.JSON200
	ch <- prometheus.MustNewConstMetric(c.reservesTotal, prometheus.GaugeValue, float64(cnts.Normal), "normal")
	ch <- prometheus.MustNewConstMetric(c.reservesTotal, prometheus.GaugeValue, float64(cnts.Conflicts), "conflicts")
	ch <- prometheus.MustNewConstMetric(c.reservesTotal, prometheus.GaugeValue, float64(cnts.Skips), "skips")
	ch <- prometheus.MustNewConstMetric(c.reservesTotal, prometheus.GaugeValue, float64(cnts.Overlaps), "overlaps")
	return nil
}

func (c *Collector) collectRecording(ctx context.Context, ch chan<- prometheus.Metric) error {
	limit := 1
	if c.enableRecordingInfo {
		limit = 20
	}
	isHalfWidth := false
	resp, err := c.client.GetRecordingWithResponse(ctx, &epgstation.GetRecordingParams{
		Limit:       &limit,
		IsHalfWidth: isHalfWidth,
	})
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return fmt.Errorf("unexpected status: %s", resp.Status())
	}

	ch <- prometheus.MustNewConstMetric(c.recordingTotal, prometheus.GaugeValue, float64(resp.JSON200.Total))
	if !c.enableRecordingInfo {
		return nil
	}

	// チャンネル名マップを取得（TTL付きキャッシュ）
	var channelNames map[int]string
	if len(resp.JSON200.Records) > 0 {
		channelNames = c.getChannelNames(ctx)
	}

	for _, item := range resp.JSON200.Records {
		idStr := strconv.Itoa(item.Id)
		channelIDStr := ""
		channelName := ""
		if item.ChannelId != nil {
			channelIDStr = strconv.Itoa(*item.ChannelId)
			if name, ok := channelNames[*item.ChannelId]; ok {
				channelName = name
			}
		}
		startAt := strconv.FormatInt(int64(item.StartAt)/1000, 10)
		endAt := strconv.FormatInt(int64(item.EndAt)/1000, 10)
		genre := ""
		if item.Genre1 != nil {
			genre = epgstation.GenreName(*item.Genre1)
		}
		ch <- prometheus.MustNewConstMetric(
			c.recordingInfo,
			prometheus.GaugeValue,
			1,
			idStr, item.Name, channelIDStr, channelName, startAt, endAt, genre,
		)
	}
	return nil
}

func (c *Collector) collectStorages(ctx context.Context, ch chan<- prometheus.Metric) error {
	resp, err := c.client.GetStoragesWithResponse(ctx)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return fmt.Errorf("unexpected status: %s", resp.Status())
	}

	for _, item := range resp.JSON200.Items {
		ch <- prometheus.MustNewConstMetric(c.storageAvailable, prometheus.GaugeValue, float64(item.Available), item.Name)
		ch <- prometheus.MustNewConstMetric(c.storageUsed, prometheus.GaugeValue, float64(item.Used), item.Name)
		ch <- prometheus.MustNewConstMetric(c.storageTotal, prometheus.GaugeValue, float64(item.Total), item.Name)
	}
	return nil
}

func (c *Collector) collectEncode(ctx context.Context, ch chan<- prometheus.Metric) error {
	isHalfWidth := false
	resp, err := c.client.GetEncodeWithResponse(ctx, &epgstation.GetEncodeParams{
		IsHalfWidth: isHalfWidth,
	})
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return fmt.Errorf("unexpected status: %s", resp.Status())
	}

	ch <- prometheus.MustNewConstMetric(c.encodeRunning, prometheus.GaugeValue, float64(len(resp.JSON200.RunningItems)))
	ch <- prometheus.MustNewConstMetric(c.encodeWaiting, prometheus.GaugeValue, float64(len(resp.JSON200.WaitItems)))
	return nil
}

func (c *Collector) collectStreams(ctx context.Context, ch chan<- prometheus.Metric) error {
	isHalfWidth := false
	resp, err := c.client.GetStreamsWithResponse(ctx, &epgstation.GetStreamsParams{
		IsHalfWidth: isHalfWidth,
	})
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return fmt.Errorf("unexpected status: %s", resp.Status())
	}

	counts := map[string]float64{
		string(epgstation.LiveStream):     0,
		string(epgstation.LiveHLS):        0,
		string(epgstation.RecordedStream): 0,
		string(epgstation.RecordedHLS):    0,
	}
	for _, item := range resp.JSON200.Items {
		counts[string(item.Type)]++
	}
	for streamType, count := range counts {
		ch <- prometheus.MustNewConstMetric(c.streamsTotal, prometheus.GaugeValue, count, streamType)
	}
	return nil
}

func (c *Collector) collectRecorded(ctx context.Context, ch chan<- prometheus.Metric) error {
	limit := 1
	isHalfWidth := false
	resp, err := c.client.GetRecordedWithResponse(ctx, &epgstation.GetRecordedParams{
		Limit:       &limit,
		IsHalfWidth: isHalfWidth,
	})
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	if resp.StatusCode() != http.StatusOK || resp.JSON200 == nil {
		return fmt.Errorf("unexpected status: %s", resp.Status())
	}

	ch <- prometheus.MustNewConstMetric(c.recordedTotal, prometheus.GaugeValue, float64(resp.JSON200.Total))
	return nil
}

func (c *Collector) collectRules(ctx context.Context, ch chan<- prometheus.Metric) error {
	// type=all を指定すると reservesCnt が返る (EPGStation の仕様)
	var rulesExt epgstation.RulesExtended
	limit := 500
	offset := 0
	for {
		reqURL := fmt.Sprintf("%s/rules?type=all&limit=%d&offset=%d", c.apiURL, limit, offset)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("API request failed: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("unexpected status: %s", resp.Status)
		}
		var page epgstation.RulesExtended
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return fmt.Errorf("failed to decode rules response: %w", err)
		}
		resp.Body.Close()
		rulesExt.Rules = append(rulesExt.Rules, page.Rules...)
		offset += len(page.Rules)
		if offset >= page.Total || len(page.Rules) == 0 {
			break
		}
	}

	disabledTotal, enabledTotal := 0, 0
	for _, rule := range rulesExt.Rules {
		if !rule.Enable {
			disabledTotal++
			continue
		}
		enabledTotal++
		name := strconv.Itoa(rule.ID)
		if rule.RuleName != nil && *rule.RuleName != "" {
			name = *rule.RuleName
		} else if rule.Keyword != nil && *rule.Keyword != "" {
			name = *rule.Keyword
		}
		count := 0
		if rule.ReservesCnt != nil {
			count = *rule.ReservesCnt
		}
		ch <- prometheus.MustNewConstMetric(
			c.ruleReservesTotal,
			prometheus.GaugeValue,
			float64(count),
			strconv.Itoa(rule.ID),
			name,
		)
	}
	ch <- prometheus.MustNewConstMetric(c.rulesTotal, prometheus.GaugeValue, float64(enabledTotal), "enabled")
	ch <- prometheus.MustNewConstMetric(c.rulesTotal, prometheus.GaugeValue, float64(disabledTotal), "disabled")

	return nil
}
