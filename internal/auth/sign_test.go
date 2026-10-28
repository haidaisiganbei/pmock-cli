package auth

import (
	"testing"
	"time"
)

func TestSignVectors(t *testing.T) {
	cases := []struct {
		name string
		at   time.Time
		ts   int64
		sign string
	}{
		{
			name: "floors to minute",
			at:   time.Unix(1700000000, 0),
			ts:   1699999980,
			sign: "cQNlhfi+m80puC3NXRSfTuVn15X+ZIEf0VwTdbvdgPc=",
		},
		{
			name: "exact minute",
			at:   time.Unix(1700000040, 0),
			ts:   1700000040,
			sign: "TyOdzapW/L05YTYUlo+HgeVM0UTTs4tkVvymMqbJ2PA=",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts, sign := Sign("AKID", "SECRET", tc.at)
			if ts != tc.ts {
				t.Fatalf("timestamp = %d, want %d", ts, tc.ts)
			}
			if sign != tc.sign {
				t.Fatalf("sign = %q, want %q", sign, tc.sign)
			}
		})
	}
}

func TestTimestampIsStableWithinMinute(t *testing.T) {
	base := time.Unix(1700000000, 0)
	if Timestamp(base) != Timestamp(base.Add(39*time.Second)) {
		t.Fatal("timestamp should not change within the same minute")
	}
	if Timestamp(base) == Timestamp(base.Add(40*time.Second)) {
		t.Fatal("timestamp should change at the next minute")
	}
}
