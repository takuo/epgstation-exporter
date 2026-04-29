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
		"/recorded": jsonHandler(t, epgstation.Records{
			Records: []epgstation.RecordedItem{},
			Total:   42,
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
				{ID: 1, RuleName: new("ルールA"), ReserveOption: epgstation.ReserveOption{Enable: true}, ReservesCnt: new(3)},
				{ID: 2, RuleName: new("ルールB"), ReserveOption: epgstation.ReserveOption{Enable: true}},
				{ID: 3, RuleName: new("ルールC"), ReserveOption: epgstation.ReserveOption{Enable: false}},
			},
			Total: 3,
		}),
	}
}

func newCollectorFromServer(t *testing.T, server *httptest.Server, enableStorage bool) *collector.Collector {
	t.Helper()
	return newCollectorFromServerWithOptions(t, server, enableStorage, false)
}

func newCollectorFromServerWithOptions(t *testing.T, server *httptest.Server, enableStorage bool, enableRecordingInfo bool) *collector.Collector {
	t.Helper()
	client, err := epgstation.NewClientWithResponses(server.URL)
	require.NoError(t, err)
	return collector.NewWithClient(client, server.URL, enableStorage, true, enableRecordingInfo)
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
			require.Len(t, labels, 2)

			labelValues := make(map[string]string, len(labels))
			for _, label := range labels {
				labelValues[label.GetName()] = label.GetValue()
			}

			version, ok := labelValues["version"]
			require.True(t, ok, "version label should be present")
			assert.Equal(t, "2.10.0", version)

			url, ok := labelValues["url"]
			require.True(t, ok, "url label should be present")
			assert.Equal(t, server.URL, url)
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

func TestCollect_RecordingInfo(t *testing.T) {
	channelID := 101
	genre := epgstation.ProgramGenreLv1(3)
	startAt := epgstation.UnixtimeMS(1713601800000)
	endAt := epgstation.UnixtimeMS(1713603600000)

	handlers := defaultHandlers(t)
	handlers["/channels"] = jsonHandler(t, epgstation.ChannelItems{
		{
			Channel:       "27",
			ChannelType:   epgstation.GR,
			HalfWidthName: "TEST",
			HasLogoData:   false,
			Id:            channelID,
			Name:          "テストチャンネル",
			NetworkId:     1,
			ServiceId:     1,
		},
	})
	handlers["/recording"] = jsonHandler(t, epgstation.Records{
		Records: []epgstation.RecordedItem{
			{
				Id:          10,
				Name:        "番組A",
				ChannelId:   &channelID,
				StartAt:     startAt,
				EndAt:       endAt,
				Genre1:      &genre,
				IsRecording: true,
			},
		},
		Total: 1,
	})

	server := newMockServer(t, handlers)
	defer server.Close()

	c := newCollectorFromServerWithOptions(t, server, false, true)

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range mfs {
		if mf.GetName() != "epgstation_recording_info" {
			continue
		}

		require.Len(t, mf.GetMetric(), 1)
		metric := mf.GetMetric()[0]
		assert.Equal(t, float64(1), metric.GetGauge().GetValue())

		labelValues := map[string]string{}
		for _, label := range metric.GetLabel() {
			labelValues[label.GetName()] = label.GetValue()
		}

		assert.Equal(t, "10", labelValues["id"])
		assert.Equal(t, "番組A", labelValues["title"])
		assert.Equal(t, "101", labelValues["channel_id"])
		assert.Equal(t, "テストチャンネル", labelValues["channel_name"])
		assert.Equal(t, "1713601800", labelValues["start_at"])
		assert.Equal(t, "1713603600", labelValues["end_at"])
		assert.Equal(t, "ドラマ", labelValues["genre"])
		found = true
	}

	assert.True(t, found, "epgstation_recording_info metric not found")
}

func TestCollect_RecordingInfoDisabled(t *testing.T) {
	channelID := 201
	handlers := defaultHandlers(t)
	handlers["/channels"] = jsonHandler(t, epgstation.ChannelItems{
		{
			Channel:       "11",
			ChannelType:   epgstation.GR,
			HalfWidthName: "TEST2",
			HasLogoData:   false,
			Id:            channelID,
			Name:          "無効化チャンネル",
			NetworkId:     2,
			ServiceId:     2,
		},
	})
	handlers["/recording"] = jsonHandler(t, epgstation.Records{
		Records: []epgstation.RecordedItem{
			{
				Id:          20,
				Name:        "番組B",
				ChannelId:   &channelID,
				StartAt:     epgstation.UnixtimeMS(1713601800000),
				EndAt:       epgstation.UnixtimeMS(1713603600000),
				IsRecording: true,
			},
		},
		Total: 1,
	})

	server := newMockServer(t, handlers)
	defer server.Close()

	c := newCollectorFromServerWithOptions(t, server, false, false)

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "epgstation_recording_info" {
			t.Fatal("epgstation_recording_info should not be present when enableRecordingInfo=false")
		}
	}
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
	c := collector.NewWithClient(nil, "", true, true, true)
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

	var rulesTotal, disabledTotal float64
	ruleReserves := map[string]float64{} // key: id

	for _, mf := range mfs {
		switch mf.GetName() {
		case "epgstation_rules_total":
			require.Len(t, mf.GetMetric(), 2)
			var foundEnabled, foundDisabled bool
			for _, m := range mf.GetMetric() {
				var state string
				for _, l := range m.GetLabel() {
					if l.GetName() == "state" {
						state = l.GetValue()
						break
					}
				}
				require.NotEmpty(t, state, "epgstation_rules_total metric missing state label")
				switch state {
				case "enabled":
					rulesTotal = m.GetGauge().GetValue()
					foundEnabled = true
				case "disabled":
					disabledTotal = m.GetGauge().GetValue()
					foundDisabled = true
				}
			}
			require.True(t, foundEnabled, "epgstation_rules_total metric with state=enabled not found")
			require.True(t, foundDisabled, "epgstation_rules_total metric with state=disabled not found")
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
	assert.Equal(t, float64(1), disabledTotal)
	assert.Equal(t, float64(3), ruleReserves["1"])
	assert.Equal(t, float64(0), ruleReserves["2"])
}

func TestCollect_RecordedTotal(t *testing.T) {
	server := newMockServer(t, defaultHandlers(t))
	defer server.Close()

	c := newCollectorFromServer(t, server, false)

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range mfs {
		if mf.GetName() == "epgstation_recorded_total" {
			require.Len(t, mf.GetMetric(), 1)
			assert.Equal(t, float64(42), mf.GetMetric()[0].GetGauge().GetValue())
			return
		}
	}
	t.Fatal("epgstation_recorded_total metric not found")
}

// コンパイル時にCollectorがprometheus.Collectorインターフェースを実装していることを確認
var _ prometheus.Collector = (*collector.Collector)(nil)

// NewWithClientが使えることを確認
var _ = func() {
	client, _ := epgstation.NewClientWithResponses("http://localhost:8888/api")
	_ = collector.NewWithClient(client, "http://localhost:8888/api", true, true, true)
	_ = context.Background()
}
