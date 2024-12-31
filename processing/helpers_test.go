package processing

import (
	"encoding/json"
	"testing"
)

func TestHelpers_ColorToHex(t *testing.T) {
	bg := "black"

	hex := ColorToHex(bg)
	t.Log(hex)
}

func TestHelpers_ParseArgs(t *testing.T) {
	args := `w=[1] iphone=[true]
b=0XFFFFFF`

	emojiArgs, err := ParseArgs(args)
	if err != nil {
		t.Error(err)
	}
	j, _ := json.MarshalIndent(emojiArgs, "", "  ")
	t.Log(string(j))
}
