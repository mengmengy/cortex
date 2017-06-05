package chunk

import (
	"context"
	"fmt"
	"sync"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/weaveworks/cortex/pkg/util"
)

// LazySeriesIterator is a struct and not just a renamed type because otherwise the Metric
// field and Metric() methods would clash.
type LazySeriesIterator struct {
	// The metric corresponding to the iterator.
	metric   model.Metric
	from     model.Time
	through  model.Time
	matchers []*metric.LabelMatcher

	// The store used to fetch chunks and samples.
	chunkStore *Store
	// The sampleSeriesIterator is created on the first sample request. This
	// does not happen with promQL queries which do not require sample data to
	// be fetched. Use sync.Once to ensure the iterator is only created once.
	sampleSeriesIterator *local.SeriesIterator
	onceCreateIterator   sync.Once
}

// NewLazySeriesIterator creates a LazySeriesIterator.
func NewLazySeriesIterator(chunkStore *Store, metric model.Metric, from model.Time, through model.Time, matchers []*metric.LabelMatcher) LazySeriesIterator {
	return LazySeriesIterator{
		chunkStore: chunkStore,
		metric:     metric,
		from:       from,
		through:    through,
		matchers:   matchers,
	}
}

// Metric implements the SeriesIterator interface.
func (it *LazySeriesIterator) Metric() metric.Metric {
	return metric.Metric{Metric: it.metric}
}

// ValueAtOrBeforeTime implements the SeriesIterator interface.
func (it *LazySeriesIterator) ValueAtOrBeforeTime(t model.Time) model.SamplePair {
	var err error
	it.onceCreateIterator.Do(func() {
		err = it.createSampleSeriesIterator()
	})
	if err != nil {
		// TODO: Handle error.
		return model.ZeroSamplePair
	}
	return (*it.sampleSeriesIterator).ValueAtOrBeforeTime(t)
}

// RangeValues implements the SeriesIterator interface.
func (it *LazySeriesIterator) RangeValues(in metric.Interval) []model.SamplePair {
	var err error
	it.onceCreateIterator.Do(func() {
		err = it.createSampleSeriesIterator()
	})
	if err != nil {
		// TODO: Handle error.
		return nil
	}
	return (*it.sampleSeriesIterator).RangeValues(in)
}

// Close implements the SeriesIterator interface.
func (it *LazySeriesIterator) Close() {}

func (it *LazySeriesIterator) createSampleSeriesIterator() error {
	metricName, ok := it.metric[model.MetricNameLabel]
	if !ok {
		return fmt.Errorf("series does not have a metric name")
	}

	ctx := context.Background()
	filters, matchers := util.SplitFiltersAndMatchers(it.matchers)
	sampleSeriesIterators, err := it.chunkStore.getMetricNameIterators(ctx, it.from, it.through, filters, matchers, metricName)
	if err != nil {
		return err
	}

	// We should only expect one sampleSeriesIterator because we are dealing
	// with one series.
	if len(sampleSeriesIterators) != 1 {
		return fmt.Errorf("multiple series found in LazySeriesIterator chunks")
	}

	it.sampleSeriesIterator = &sampleSeriesIterators[0]
	return nil
}
