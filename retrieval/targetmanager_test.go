// Copyright 2013 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package retrieval

import (
	"reflect"
	"testing"

	"golang.org/x/net/context"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage/local"
)

func TestTargetSetRecreatesTargetGroupsEveryRun(t *testing.T) {

	verifyPresence := func(tgroups map[string][]*Target, name string, present bool) {
		if _, ok := tgroups[name]; ok != present {
			msg := ""
			if !present {
				msg = "not "
			}
			t.Fatalf("'%s' should %sbe present in TargetSet.tgroups: %s", name, msg, tgroups)
		}

	}

	scrapeConfig := &config.ScrapeConfig{}

	sOne := `
job_name: "foo"
dns_sd_configs:
- names:
  - "srv.name.one.example.org"
`
	if err := yaml.Unmarshal([]byte(sOne), scrapeConfig); err != nil {
		t.Fatalf("Unable to load YAML config sOne: %s", err)
	}

	// Not properly setting it up, but that seems okay
	mss := &local.MemorySeriesStorage{}

	ts := newTargetSet(scrapeConfig, mss)

	ts.runProviders(context.Background(), providersFromConfig(scrapeConfig))

	verifyPresence(ts.tgroups, "dns/0/srv.name.one.example.org", true)

	sTwo := `
job_name: "foo"
dns_sd_configs:
- names:
  - "srv.name.two.example.org"
`
	if err := yaml.Unmarshal([]byte(sTwo), scrapeConfig); err != nil {
		t.Fatalf("Unable to load YAML config sTwo: %s", err)
	}

	ts.runProviders(context.Background(), providersFromConfig(scrapeConfig))

	verifyPresence(ts.tgroups, "dns/0/srv.name.one.example.org", false)
	verifyPresence(ts.tgroups, "dns/0/srv.name.two.example.org", true)
}

func mustNewRegexp(s string) config.Regexp {
	re, err := config.NewRegexp(s)
	if err != nil {
		panic(err)
	}
	return re
}

func TestPopulateLabels(t *testing.T) {
	cases := []struct {
		in      model.LabelSet
		cfg     *config.ScrapeConfig
		res     model.LabelSet
		resOrig model.LabelSet
	}{
		// Regular population of scrape config options.
		{
			in: model.LabelSet{
				model.AddressLabel: "1.2.3.4:1000",
				"custom":           "value",
			},
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
			},
			res: model.LabelSet{
				model.AddressLabel:     "1.2.3.4:1000",
				model.InstanceLabel:    "1.2.3.4:1000",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
				"custom":               "value",
			},
			resOrig: model.LabelSet{
				model.AddressLabel:     "1.2.3.4:1000",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
				"custom":               "value",
			},
		},
		// Pre-define/overwrite scrape config labels.
		// Leave out port and expect it to be defaulted to scheme.
		{
			in: model.LabelSet{
				model.AddressLabel:     "1.2.3.4",
				model.SchemeLabel:      "http",
				model.MetricsPathLabel: "/custom",
				model.JobLabel:         "custom-job",
			},
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
			},
			res: model.LabelSet{
				model.AddressLabel:     "1.2.3.4:80",
				model.InstanceLabel:    "1.2.3.4:80",
				model.SchemeLabel:      "http",
				model.MetricsPathLabel: "/custom",
				model.JobLabel:         "custom-job",
			},
			resOrig: model.LabelSet{
				model.AddressLabel:     "1.2.3.4",
				model.SchemeLabel:      "http",
				model.MetricsPathLabel: "/custom",
				model.JobLabel:         "custom-job",
			},
		},
		// Provide instance label. HTTPS port default for IPv6.
		{
			in: model.LabelSet{
				model.AddressLabel:  "[::1]",
				model.InstanceLabel: "custom-instance",
			},
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
			},
			res: model.LabelSet{
				model.AddressLabel:     "[::1]:443",
				model.InstanceLabel:    "custom-instance",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
			},
			resOrig: model.LabelSet{
				model.AddressLabel:     "[::1]",
				model.InstanceLabel:    "custom-instance",
				model.SchemeLabel:      "https",
				model.MetricsPathLabel: "/metrics",
				model.JobLabel:         "job",
			},
		},
		// Apply relabeling.
		{
			in: model.LabelSet{
				model.AddressLabel: "1.2.3.4:1000",
				"custom":           "value",
			},
			cfg: &config.ScrapeConfig{
				Scheme:      "https",
				MetricsPath: "/metrics",
				JobName:     "job",
				RelabelConfigs: []*config.RelabelConfig{
					{
						Action:       config.RelabelDrop,
						Regex:        mustNewRegexp(".*"),
						SourceLabels: model.LabelNames{"job"},
					},
				},
			},
			res:     nil,
			resOrig: nil,
		},
	}
	for i, c := range cases {
		res, orig, err := populateLabels(c.in, c.cfg)
		if err != nil {
			t.Fatalf("case %d: %s", i, err)
		}
		if !reflect.DeepEqual(res, c.res) {
			t.Errorf("case %d: expected res\n\t%+v\n got\n\t%+v", i, c.res, res)
		}
		if !reflect.DeepEqual(orig, c.resOrig) {
			t.Errorf("case %d: expected resOrig\n\t%+v\n got\n\t%+v", i, c.resOrig, orig)
		}
	}
}
