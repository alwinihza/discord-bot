package commissions

import "testing"

func TestCustomIDRoundTrip(t *testing.T) {
	a := Action{ID: "7b1f3c52-0a4e-4c1b-9d57-2f6a7e1d9a10", Status: "deposit_paid"}
	if got, ok := ParseButtonID(ButtonID(a)); !ok || got != a {
		t.Errorf("button round trip = %+v, %v", got, ok)
	}
	if got, ok := ParseModalID(ModalID(a)); !ok || got != a {
		t.Errorf("modal round trip = %+v, %v", got, ok)
	}
	if len(ModalID(a)) > 100 || len(ButtonID(a)) > 100 {
		t.Error("custom id over Discord's 100 character limit")
	}
	if _, ok := ParseButtonID(ModalID(a)); ok {
		t.Error("a modal id must not parse as a button id")
	}
}

func TestCustomIDRejectsMalformed(t *testing.T) {
	for _, id := range []string{
		"", "commission:", "commission:c1", "commission:c1:", "commission::quoted",
		"commission:c1:Quoted", "commission:c1:quoted:extra", "commission:a/b:quoted", "playlist:c1:quoted",
	} {
		if a, ok := ParseButtonID(id); ok {
			t.Errorf("ParseButtonID(%q) = %+v, want rejection", id, a)
		}
	}
}

func TestOwnsComponent(t *testing.T) {
	for id, want := range map[string]bool{"commission:x": true, "commission-note:x": true, "playlist:confirm:x": false, "random": false} {
		if OwnsComponent(id) != want {
			t.Errorf("OwnsComponent(%q) = %v", id, !want)
		}
	}
}

func TestParseManagerIDs(t *testing.T) {
	got, bad := ParseManagerIDs(" 100000000000000001, ,100000000000000002,nope,1234 ")
	if len(got) != 2 || !got["100000000000000001"] || !got["100000000000000002"] {
		t.Errorf("managers = %v", got)
	}
	if len(bad) != 2 || bad[0] != "nope" || bad[1] != "1234" {
		t.Errorf("bad = %v", bad)
	}
	if got, bad := ParseManagerIDs(""); len(got) != 0 || len(bad) != 0 {
		t.Errorf("empty = %v %v", got, bad)
	}
}
