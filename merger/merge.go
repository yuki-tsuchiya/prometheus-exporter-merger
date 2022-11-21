package merger

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"

	prom "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"golang.org/x/sync/errgroup"
)

func (m *merger) merge(ctx context.Context, w io.Writer) error {

	mu := &sync.Mutex{}
	result := map[string]*prom.MetricFamily{}

	g, ctx := errgroup.WithContext(ctx)
	for _, source := range m.sources {
		source := source
		g.Go(func() error {
			resp, err := m.client.Get(source.url)
			if err != nil {
				fmt.Printf("get url: %s", source.url)
				return nil
			}
			defer resp.Body.Close()
			tp := new(expfmt.TextParser)
			out, err := tp.TextToMetricFamilies(resp.Body)
			if err != nil {
				fmt.Printf("parse url: %s", source.url)
				return nil
			}
			mu.Lock()
			defer mu.Unlock()
			for name, metricFamily := range out {
				// append labels
				if len(source.labels) > 0 {
					for _, metric := range metricFamily.Metric {
						metric.Label = append(metric.Label, source.labels...)
					}
				}
				// append metrics
				if mfResult, ok := result[name]; ok {
					mfResult.Metric = append(mfResult.Metric, metricFamily.Metric...)
				} else {
					result[name] = metricFamily
				}
			}
			return nil
		})
	}

	// wait to process all routines
	if err := g.Wait(); err != nil {
		return err
	}

	// sort names
	var names []string
	for n := range result {
		names = append(names, n)
	}
	sort.Strings(names)

	// write result
	enc := expfmt.NewEncoder(w, expfmt.FmtText)
	for _, n := range names {
		err := enc.Encode(result[n])
		if err != nil {
			return err
		}
	}
	return nil
}
