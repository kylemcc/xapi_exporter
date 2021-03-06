package main

import (
	"reflect"
	"testing"

	xenAPI "github.com/johnprather/go-xen-api-client"
)

func TestParseLegendEntry(t *testing.T) {
	cases := []struct {
		input     string
		expected  Entry
		expectErr bool
	}{
		{"AVERAGE:vm:15f9d56e-938a-34fc-73f3-a7e08a0445eb:vbd_xvdd_io_throughput_write", Entry{"AVERAGE", "vm", "15f9d56e-938a-34fc-73f3-a7e08a0445eb", "vbd_xvdd_io_throughput_write"}, false},
		{"garbage", Entry{}, true},
	}

	for _, c := range cases {
		actual, err := parseLegendEntry(c.input)
		if err != nil && !c.expectErr {
			t.Errorf("unexpected error parsing entry [%v]: %v", c.input, err)
		}
		if err == nil && c.expectErr {
			t.Errorf("expected error for input [%v]", c.input)
		}
		if actual != c.expected {
			t.Errorf("parsing legend entry failed. got [%+v] expected [%+v]", actual, c.expected)
		}
	}
}

func TestMappingRrds(t *testing.T) {
	rrdMetrics := []*RrdUpdates{
		&RrdUpdates{
			RrdMeta{
				1495554585,
				3,
				Legend{
					[]Entry{
						Entry{"AVERAGE", "VM", "1111-111", "CPU"},
						Entry{"AVERAGE", "VM", "1111-111", "MEMORY"},
						Entry{"AVERAGE", "HOST", "555-555", "CPU"},
					},
				},
			},
			Data{
				[]Row{
					{
						Timestamp: 1495554585,
						Values:    []float64{1.1, 2.2, 8.8},
					},
				},
			},
		},
		&RrdUpdates{
			RrdMeta{
				1495554585,
				5,
				Legend{
					[]Entry{
						Entry{},
						Entry{},
						Entry{},
						Entry{},
						Entry{},
					},
				},
			},
			Data{
				[]Row{
					{
						Timestamp: 1495554585,
						Values:    []float64{1.1, 2.2, 3.3, 4.4, 5.5},
					},
				},
			},
		},
	}

	hostRecords := map[xenAPI.HostRef]xenAPI.HostRecord{}
	vmRecords := map[xenAPI.VMRef]xenAPI.VMRecord{}

	mapped := mapRrds(rrdMetrics, hostRecords, vmRecords)

	var expectedLen int
	for _, u := range rrdMetrics {
		expectedLen += len(u.Meta.Legend.Entries)
	}

	if length := len(mapped); length != expectedLen {
		t.Errorf("Expected 1 element but got %d", length)
	}
}

func TestParseCpuMetric(t *testing.T) {
	cases := []struct {
		input          string
		expectedName   string
		expectedLabels map[string]string
	}{
		{"cpu0", "cpu", map[string]string{"cpu_num": "0"}},
		{"cpu1", "cpu", map[string]string{"cpu_num": "1"}},
		{"cpu0-C0", "cpu", map[string]string{"cpu_num": "0", "core_num": "0"}},
		{"cpu0-C1", "cpu", map[string]string{"cpu_num": "0", "core_num": "1"}},
		{"cpu1-C0", "cpu", map[string]string{"cpu_num": "1", "core_num": "0"}},
		{"cpu1-C1", "cpu", map[string]string{"cpu_num": "1", "core_num": "1"}},
	}

	for _, c := range cases {
		actualName, actualLabels := parseCpuMetric(c.input)
		if actualName != c.expectedName {
			t.Errorf("failed to parse cpu metric name. Got [%v] expected [%v]", actualName, c.expectedName)
		}
		if !reflect.DeepEqual(actualLabels, c.expectedLabels) {
			t.Errorf("failed to parse cpu metric labels. Got [%v] expected [%v]", actualLabels, c.expectedLabels)
		}
	}
}
