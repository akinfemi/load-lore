package power

import "testing"

func TestParse(t *testing.T) {
	const ac = "Now drawing from 'AC Power'\n -InternalBattery-0 (id=22544483)\t100%; charged; 0:00 remaining present: true\n"
	const batt = "Now drawing from 'Battery Power'\n -InternalBattery-0 (id=22544483)\t81%; discharging; 5:10 remaining present: true\n"
	const lpmOff = "System-wide power settings:\nCurrently in use:\n standby              1\n lowpowermode         0\n womp                 1\n"
	const lpmOn = "Currently in use:\n lowpowermode         1\n"
	for _, tc := range []struct {
		batt, settings string
		want           State
	}{
		{ac, lpmOff, State{Known: true}},
		{batt, lpmOn, State{Known: true, OnBattery: true, LowPower: true}},
		{ac, lpmOn, State{Known: true, LowPower: true}},
		{"No batteries found\n", lpmOff, State{}}, // a desktop's pmset may say nothing of power source
	} {
		if got := Parse(tc.batt, tc.settings); got != tc.want {
			t.Errorf("Parse(%q, %q) = %+v, want %+v", tc.batt, tc.settings, got, tc.want)
		}
	}
}
