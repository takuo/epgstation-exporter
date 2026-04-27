package collector_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/takuo/epgstation-exporter/internal/collector"
	"github.com/takuo/epgstation-exporter/pkg/epgstation"
)

// モックサーバーを作成するヘルパー
func newMockServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, handler := range handlers {
		mux.HandleFunc(path, handler)
	}
	return httptest.NewServer(mux)
}

func jsonHandler(t *testing.T, v any) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(v))
	}
}

func defaultHandlers(t *testing.T) map[string]http.HandlerFunc {
	t.Helper()
	return map[string]http.HandlerFunc{
		"/version": jsonHandler(t, epgstation.Version{Version: "2.10.0"}),
		"/reserves/cnts": jsonHandler(t, epgstation.ReserveCnts{
			Normal:    10,
			Conflicts: 1,
			Skips:     2,
			Overlaps:  0,
		}),
		"/recording": jsonHandler(t, epgstation.Records{
			Records: []epgstation.RecordedItem{},
			Total:   3,
		}),
		"/storages": jsonHandler(t, epgstation.StorageInfo{
			Items: []epgstation.StorageItem{
				{Name: "disk1", Available: 500 * 1024 * 1024 * 1024, Used: 500 * 1024 * 1024 * 1024, Total: 1000 * 1024 * 1024 * 1024},
			},
		}),
		"/encode": jsonHandler(t, epgstation.EncodeInfo{
			RunningItems: []epgstation.EncodeProgramItem{},
			WaitItems:    []epgstation.EncodeProgramItem{},
		}),
		"/streams": jsonHandler(t, epgstation.StreamInfo{
			Items: []epgstation.StreamInfoItem{},
		}),
		"/rules": jsonHandler(t, epgstation.RulesExtended{
			Rules: []epgstation.RuleItem{
				{ID: 1, RuleName: new("ルールA"), ReservesCnt: new(3)},
				{ID: 2, RuleName: new("ルールB"), ReservesCnt: nil},
			},
			Total: 2,
		}),
	}
}

func newCollectorFromServer(t *testing.T, server *httptest.Server, enableStorage bool) *collector.Collector {
	t.Helper()
	client, err := epgstation.NewClientWithResponses(server.URL)
	require.NoError(t, err)
	return collector.NewWithClient(client, server.URL, enableStorage, true)
}

func TestCollect_Up(t *testing.T) {
	server := newMockServer(t, defaultHandlers(t))
	defer server.Close()

	c := newCollectorFromServer(t, server, false)
	count := testutil.CollectAndCount(c)
	assert.Greater(t, count, 0)
}

func TestCollect_VersionInfo(t *testing.T) {
	server := newMockServer(t, defaultHandlers(t))
	defer server.Close()

	c := newCollectorFromServer(t, server, false)

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	var foundUp, foundInfo bool
	for _, mf := range mfs {
		switch mf.GetName() {
		case "epgstation_up":
			foundUp = true
			require.Len(t, mf.GetMetric(), 1)
			assert.Equal(t, float64(1), mf.GetMetric()[0].GetGauge().GetValue())
		case "epgstation_info":
			foundInfo = true
			require.Len(t, mf.GetMetric(), 1)
			assert.Equal(t, float64(1), mf.GetMetric()[0].GetGauge().GetValue())
			labels := mf.GetMetric()[0].GetLabel()
			require.Len(t, labels, 1)
			assert.Equal(t, "version", labels[0].GetName())
			assert.Equal(t, "2.10.0", labels[0].GetValue())
		}
	}
	assert.True(t, foundUp, "epgstation_up metric should be present")
	assert.True(t, foundInfo, "epgstation_info metric should be present")
}

func TestCollect_ReservesCnts(t *testing.T) {
	server := newMockServer(t, defaultHandlers(t))
	defer server.Close()

	c := newCollectorFromServer(t, server, false)

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	reserveValues := map[string]float64{}
	for _, mf := range mfs {
		if mf.GetName() == "epgstation_reserves_total" {
			for _, m := range mf.GetMetric() {
				for _, l := range m.GetLabel() {
					if l.GetName() == "type" {
						reserveValues[l.GetValue()] = m.GetGauge().GetValue()
					}
				}
			}
		}
	}

	assert.Equal(t, float64(10), reserveValues["normal"])
	assert.Equal(t, float64(1), reserveValues["conflicts"])
	assert.Equal(t, float64(2), reserveValues["skips"])
	assert.Equal(t, float64(0), reserveValues["overlaps"])
}

func TestCollect_Recording(t *testing.T) {
	server := newMockServer(t, defaultHandlers(t))
	defer server.Close()

	c := newCollectorFromServer(t, server, false)

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "epgstation_recording_total" {
			require.Len(t, mf.GetMetric(), 1)
			assert.Equal(t, float64(3), mf.GetMetric()[0].GetGauge().GetValue())
			return
		}
	}
	t.Fatal("epgstation_recording_total metric not found")
}

func TestCollect_StorageMetrics(t *testing.T) {
	server := newMockServer(t, defaultHandlers(t))
	defer server.Close()

	c := newCollectorFromServer(t, server, true)

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	storageMetrics := map[string]float64{}
	for _, mf := range mfs {
		switch mf.GetName() {
		case "epgstation_storage_available_bytes",
			"epgstation_storage_used_bytes",
			"epgstation_storage_total_bytes":
			for _, m := range mf.GetMetric() {
				storageMetrics[mf.GetName()] = m.GetGauge().GetValue()
			}
		}
	}

	assert.Equal(t, float64(500*1024*1024*1024), storageMetrics["epgstation_storage_available_bytes"])
	assert.Equal(t, float64(500*1024*1024*1024), storageMetrics["epgstation_storage_used_bytes"])
	assert.Equal(t, float64(1000*1024*1024*1024), storageMetrics["epgstation_storage_total_bytes"])
}

func TestCollect_StorageDisabled(t *testing.T) {
	server := newMockServer(t, defaultHandlers(t))
	defer server.Close()

	c := newCollectorFromServer(t, server, false)

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		switch mf.GetName() {
		case "epgstation_storage_available_bytes",
			"epgstation_storage_used_bytes",
			"epgstation_storage_total_bytes":
			t.Errorf("storage metric %s should not be present when disabled", mf.GetName())
		}
	}
}

func TestCollect_EncodeMetrics(t *testing.T) {
	handlers := defaultHandlers(t)
	handlers["/encode"] = jsonHandler(t, epgstation.EncodeInfo{
		RunningItems: []epgstation.EncodeProgramItem{
			{Id: 1, Mode: "mode1"},
		},
		WaitItems: []epgstation.EncodeProgramItem{
			{Id: 2, Mode: "mode1"},
			{Id: 3, Mode: "mode1"},
		},
	})

	server := newMockServer(t, handlers)
	defer server.Close()

	c := newCollectorFromServer(t, server, false)

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	encodeMetrics := map[string]float64{}
	for _, mf := range mfs {
		switch mf.GetName() {
		case "epgstation_encode_running_total", "epgstation_encode_waiting_total":
			require.Len(t, mf.GetMetric(), 1)
			encodeMetrics[mf.GetName()] = mf.GetMetric()[0].GetGauge().GetValue()
		}
	}

	assert.Equal(t, float64(1), encodeMetrics["epgstation_encode_running_total"])
	assert.Equal(t, float64(2), encodeMetrics["epgstation_encode_waiting_total"])
}

func TestCollect_StreamMetrics(t *testing.T) {
	handlers := defaultHandlers(t)
	startAt := 0
	endAt := 0
	handlers["/streams"] = jsonHandler(t, epgstation.StreamInfo{
		Items: []epgstation.StreamInfoItem{
			{StreamId: 1, Type: epgstation.LiveStream, Mode: 0, IsEnable: true, ChannelId: 1, Name: "test1", StartAt: startAt, EndAt: endAt},
			{StreamId: 2, Type: epgstation.LiveHLS, Mode: 0, IsEnable: true, ChannelId: 2, Name: "test2", StartAt: startAt, EndAt: endAt},
		},
	})

	server := newMockServer(t, handlers)
	defer server.Close()

	c := newCollectorFromServer(t, server, false)

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	streamCounts := map[string]float64{}
	for _, mf := range mfs {
		if mf.GetName() == "epgstation_streams_total" {
			for _, m := range mf.GetMetric() {
				for _, l := range m.GetLabel() {
					if l.GetName() == "type" {
						streamCounts[l.GetValue()] = m.GetGauge().GetValue()
					}
				}
			}
		}
	}

	assert.Equal(t, float64(1), streamCounts["LiveStream"])
	assert.Equal(t, float64(1), streamCounts["LiveHLS"])
	assert.Equal(t, float64(0), streamCounts["RecordedStream"])
	assert.Equal(t, float64(0), streamCounts["RecordedHLS"])
}

func TestCollect_APIDown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := newCollectorFromServer(t, server, false)

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "epgstation_up" {
			require.Len(t, mf.GetMetric(), 1)
			assert.Equal(t, float64(0), mf.GetMetric()[0].GetGauge().GetValue())
			return
		}
	}
	t.Fatal("epgstation_up metric not found")
}

func TestDescribe(t *testing.T) {
	c := collector.NewWithClient(nil, "", true, true)
	ch := make(chan *prometheus.Desc, 20)
	c.Describe(ch)
	close(ch)

	descs := []*prometheus.Desc{}
	for d := range ch {
		descs = append(descs, d)
	}
	assert.NotEmpty(t, descs)
}

func TestCollect_RulesMetrics(t *testing.T) {
	server := newMockServer(t, defaultHandlers(t))
	defer server.Close()

	c := newCollectorFromServer(t, server, false)

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	var rulesTotal float64
	ruleReserves := map[string]float64{} // key: id

	for _, mf := range mfs {
		switch mf.GetName() {
		case "epgstation_rules_total":
			require.Len(t, mf.GetMetric(), 1)
			rulesTotal = mf.GetMetric()[0].GetGauge().GetValue()
		case "epgstation_rule_reserves_total":
			for _, m := range mf.GetMetric() {
				var id, name string
				for _, l := range m.GetLabel() {
					if l.GetName() == "id" {
						id = l.GetValue()
					}
					if l.GetName() == "name" {
						name = l.GetValue()
					}
				}
				_ = name
				ruleReserves[id] = m.GetGauge().GetValue()
			}
		}
	}

	assert.Equal(t, float64(2), rulesTotal)
	assert.Equal(t, float64(3), ruleReserves["1"])
	assert.Equal(t, float64(0), ruleReserves["2"])
}

// コンパイル時にCollectorがprometheus.Collectorインターフェースを実装していることを確認
var _ prometheus.Collector = (*collector.Collector)(nil)

// NewWithClientが使えることを確認
var _ = func() {
	client, _ := epgstation.NewClientWithResponses("http://localhost:8888/api")
	_ = collector.NewWithClient(client, "http://localhost:8888/api", true, true)
	_ = context.Background()
}
