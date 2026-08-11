package g79client

import (
	"encoding/json"
	"testing"
)

func TestSocialProfileUnmarshalAcceptsDeveloperObject(t *testing.T) {
	payload := []byte(`{
		"id": 2492760332,
		"nickname": "Happy",
		"is_developer": {
			"developer_id": "7633752159",
			"uid": "2492760332",
			"username": "eternity_hope@163.com"
		}
	}`)

	var profile SocialProfile
	if err := json.Unmarshal(payload, &profile); err != nil {
		t.Fatalf("unmarshal social profile failed: %v", err)
	}
	if profile.ID.String() != "2492760332" {
		t.Fatalf("unexpected id: %s", profile.ID.String())
	}
	if _, ok := profile.IsDeveloper.(map[string]any); !ok {
		t.Fatalf("unexpected is_developer type: %T", profile.IsDeveloper)
	}
}
