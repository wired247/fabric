/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package metrics

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"io"
	"net"
	"strings"

	"github.com/cactus/go-statsd-client/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/uber-go/tally"
	statsdreporter "github.com/uber-go/tally/statsd"
)

const statsdAddr string = "127.0.0.1:8125"

type testIntValue struct {
	val      int64
	tags     map[string]string
	reporter *testStatsReporter
}

func (m *testIntValue) ReportCount(value int64) {
	m.val = value
	m.reporter.cg.Done()
}

type testFloatValue struct {
	val      float64
	tags     map[string]string
	reporter *testStatsReporter
}

func (m *testFloatValue) ReportGauge(value float64) {
	m.val = value
	m.reporter.gg.Done()
}

type testStatsReporter struct {
	cg sync.WaitGroup
	gg sync.WaitGroup

	scope Scope

	counters map[string]*testIntValue
	gauges   map[string]*testFloatValue

	flushes int32
}

// newTestStatsReporter returns a new TestStatsReporter
func newTestStatsReporter() *testStatsReporter {
	return &testStatsReporter{
		counters: make(map[string]*testIntValue),
		gauges:   make(map[string]*testFloatValue)}
}

func (r *testStatsReporter) WaitAll() {
	r.cg.Wait()
	r.gg.Wait()
}

func (r *testStatsReporter) AllocateCounter(
	name string, tags map[string]string,
) tally.CachedCount {
	counter := &testIntValue{
		val:      0,
		tags:     tags,
		reporter: r,
	}
	r.counters[name] = counter
	return counter
}

func (r *testStatsReporter) ReportCounter(name string, tags map[string]string, value int64) {
	r.counters[name] = &testIntValue{
		val:  value,
		tags: tags,
	}
	r.cg.Done()
}

func (r *testStatsReporter) AllocateGauge(
	name string, tags map[string]string,
) tally.CachedGauge {
	gauge := &testFloatValue{
		val:      0,
		tags:     tags,
		reporter: r,
	}
	r.gauges[name] = gauge
	return gauge
}

func (r *testStatsReporter) ReportGauge(name string, tags map[string]string, value float64) {
	r.gauges[name] = &testFloatValue{
		val:  value,
		tags: tags,
	}
	r.gg.Done()
}

func (r *testStatsReporter) AllocateTimer(
	name string, tags map[string]string,
) tally.CachedTimer {
	return nil
}

func (r *testStatsReporter) ReportTimer(name string, tags map[string]string, interval time.Duration) {

}

func (r *testStatsReporter) AllocateHistogram(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
) tally.CachedHistogram {
	return nil
}

func (r *testStatsReporter) ReportHistogramValueSamples(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
	bucketLowerBound,
	bucketUpperBound float64,
	samples int64,
) {

}

func (r *testStatsReporter) ReportHistogramDurationSamples(
	name string,
	tags map[string]string,
	buckets tally.Buckets,
	bucketLowerBound,
	bucketUpperBound time.Duration,
	samples int64,
) {

}

func (r *testStatsReporter) Capabilities() tally.Capabilities {
	return nil
}

func (r *testStatsReporter) Flush() {
	atomic.AddInt32(&r.flushes, 1)
}

func TestCounter(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s, c := newRootScope(opts, 1*time.Second)
	defer c.Close()
	r.cg.Add(1)
	s.Counter("foo").Inc(1)
	r.cg.Wait()

	assert.Equal(t, int64(1), r.counters[namespace+".foo"].val)

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Should panic when wrong key used")
		}
	}()
	assert.Equal(t, int64(1), r.counters[namespace+".foo1"].val)
}

func TestMultiCounterReport(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s, c := newRootScope(opts, 2*time.Second)
	defer c.Close()
	r.cg.Add(1)
	go s.Counter("foo").Inc(1)
	go s.Counter("foo").Inc(3)
	go s.Counter("foo").Inc(5)
	r.cg.Wait()

	assert.Equal(t, int64(9), r.counters[namespace+".foo"].val)
}

func TestGauge(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s, c := newRootScope(opts, 1*time.Second)
	defer c.Close()
	r.gg.Add(1)
	s.Gauge("foo").Update(float64(1.33))
	r.gg.Wait()

	assert.Equal(t, float64(1.33), r.gauges[namespace+".foo"].val)
}

func TestMultiGaugeReport(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s, c := newRootScope(opts, 1*time.Second)
	defer c.Close()

	r.gg.Add(1)
	s.Gauge("foo").Update(float64(1.33))
	s.Gauge("foo").Update(float64(3.33))
	r.gg.Wait()

	assert.Equal(t, float64(3.33), r.gauges[namespace+".foo"].val)
}

func TestSubScope(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s, c := newRootScope(opts, 1*time.Second)
	defer c.Close()
	subs := s.SubScope("foo")

	r.gg.Add(1)
	subs.Gauge("bar").Update(float64(1.33))
	r.gg.Wait()

	assert.Equal(t, float64(1.33), r.gauges[namespace+".foo.bar"].val)

	r.cg.Add(1)
	subs.Counter("haha").Inc(1)
	r.cg.Wait()

	assert.Equal(t, int64(1), r.counters[namespace+".foo.haha"].val)
}

func TestTagged(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s, c := newRootScope(opts, 1*time.Second)
	defer c.Close()
	subs := s.Tagged(map[string]string{"env": "test"})

	r.gg.Add(1)
	subs.Gauge("bar").Update(float64(1.33))
	r.gg.Wait()

	assert.Equal(t, float64(1.33), r.gauges[namespace+".bar"].val)
	assert.EqualValues(t, map[string]string{
		"env": "test",
	}, r.gauges[namespace+".bar"].tags)

	r.cg.Add(1)
	subs.Counter("haha").Inc(1)
	r.cg.Wait()

	assert.Equal(t, int64(1), r.counters[namespace+".haha"].val)
	assert.EqualValues(t, map[string]string{
		"env": "test",
	}, r.counters[namespace+".haha"].tags)
}

func TestTaggedExistingReturnsSameScope(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()

	for _, initialTags := range []map[string]string{
		nil,
		{"env": "test"},
	} {
		root, c := newRootScope(tally.ScopeOptions{Prefix: "foo", Tags: initialTags, Reporter: r}, 0)

		rootScope := root.(*scope)
		fooScope := root.Tagged(map[string]string{"foo": "bar"}).(*scope)

		assert.NotEqual(t, rootScope, fooScope)
		assert.Equal(t, fooScope, fooScope.Tagged(nil))

		fooBarScope := fooScope.Tagged(map[string]string{"bar": "baz"}).(*scope)

		assert.NotEqual(t, fooScope, fooBarScope)
		assert.Equal(t, fooBarScope, fooScope.Tagged(map[string]string{"bar": "baz"}).(*scope))
		c.Close()
	}
}

func TestSubScopeTagged(t *testing.T) {
	t.Parallel()
	r := newTestStatsReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s, c := newRootScope(opts, 1*time.Second)
	defer c.Close()
	subs := s.SubScope("sub")
	subtags := subs.Tagged(map[string]string{"env": "test"})

	r.gg.Add(1)
	subtags.Gauge("bar").Update(float64(1.33))
	r.gg.Wait()

	assert.Equal(t, float64(1.33), r.gauges[namespace+".sub.bar"].val)
	assert.EqualValues(t, map[string]string{
		"env": "test",
	}, r.gauges[namespace+".sub.bar"].tags)

	r.cg.Add(1)
	subtags.Counter("haha").Inc(1)
	r.cg.Wait()

	assert.Equal(t, int64(1), r.counters[namespace+".sub.haha"].val)
	assert.EqualValues(t, map[string]string{
		"env": "test",
	}, r.counters[namespace+".sub.haha"].tags)
}

func TestMetricsByStatsdReporter(t *testing.T) {
	t.Parallel()
	udpAddr, err := net.ResolveUDPAddr("udp", statsdAddr)
	if err != nil {
		t.Fatal(err)
	}

	server, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	r := newTestStatsdReporter()
	opts := tally.ScopeOptions{
		Prefix:    namespace,
		Separator: tally.DefaultSeparator,
		Reporter:  r}

	s, c := newRootScope(opts, 1*time.Second)
	defer c.Close()
	subs := s.SubScope("peer").Tagged(map[string]string{"component": "committer", "env": "test"})
	subs.Counter("success_total").Inc(1)
	subs.Gauge("channel_total").Update(4)

	buffer := make([]byte, 4096)
	n, _ := io.ReadAtLeast(server, buffer, 1)
	result := string(buffer[:n])

	expected := []string{
		`hyperledger.fabric.peer.success_total.component-committer.env-test:1|c`,
		`hyperledger.fabric.peer.channel_total.component-committer.env-test:4|g`,
	}

	for i, res := range strings.Split(result, "\n") {
		if res != expected[i] {
			t.Errorf("Got `%s`, expected `%s`", res, expected[i])
		}
	}
}

func newTestStatsdReporter() tally.StatsReporter {
	statter, _ := statsd.NewBufferedClient(statsdAddr,
		"", 100*time.Millisecond, 512)

	opts := statsdreporter.Options{}
	return newStatsdReporter(statter, opts)
}
