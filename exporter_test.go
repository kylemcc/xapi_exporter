package main

import "testing"

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
			t.Errorf("expected error but did not get one")
		}
		if actual != c.expected {
			t.Errorf("parsing legend entry failed. got [%+v] expected [%+v]", actual, c.expected)
		}
	}
}
