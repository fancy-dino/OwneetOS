// SPDX-License-Identifier: GPL-3.0-or-later

package power

import (
	"errors"
	"testing"
)

func TestCheckError(t *testing.T) {
	cases := map[string]error{"yes": nil, "na": ErrUnsupported, "no": ErrNotAllowed, "challenge": ErrNotAllowed}
	for answer, want := range cases {
		if got := CheckError(answer); !errors.Is(got, want) || (want == nil && got != nil) {
			t.Errorf("CheckError(%q) = %v, want %v", answer, got, want)
		}
	}
}
