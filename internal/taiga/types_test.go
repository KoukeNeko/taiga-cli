package taiga

import (
	"encoding/json"
	"testing"
)

func TestTagUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Tag
		wantErr bool
	}{
		{name: "name and color", input: `["bug","#ff0000"]`, want: Tag{Name: "bug", Color: strPtr("#ff0000")}},
		{name: "name without color", input: `["bug",null]`, want: Tag{Name: "bug", Color: nil}},
		{name: "not a pair", input: `"bug"`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Tag
			err := json.Unmarshal([]byte(tt.input), &got)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Unmarshal(%s) error = nil, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unmarshal(%s) error = %v", tt.input, err)
			}
			if got.Name != tt.want.Name {
				t.Fatalf("Name = %q, want %q", got.Name, tt.want.Name)
			}
			if (got.Color == nil) != (tt.want.Color == nil) {
				t.Fatalf("Color = %v, want %v", got.Color, tt.want.Color)
			}
			if got.Color != nil && *got.Color != *tt.want.Color {
				t.Fatalf("Color = %q, want %q", *got.Color, *tt.want.Color)
			}
		})
	}
}

func TestTagUnmarshalJSONOverwritesExistingValue(t *testing.T) {
	got := Tag{Name: "old", Color: strPtr("#000000")}
	if err := json.Unmarshal([]byte(`[null,null]`), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got.Name != "" {
		t.Fatalf("Name = %q, want empty; a decode into a populated Tag must not keep the stale value", got.Name)
	}
	if got.Color != nil {
		t.Fatalf("Color = %v, want nil", got.Color)
	}
}

func TestTagMarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		tag  Tag
		want string
	}{
		{name: "name and color", tag: Tag{Name: "bug", Color: strPtr("#ff0000")}, want: `["bug","#ff0000"]`},
		{name: "name without color", tag: Tag{Name: "bug"}, want: `["bug",null]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.tag)
			if err != nil {
				t.Fatalf("Marshal(%#v) error = %v", tt.tag, err)
			}
			if string(data) != tt.want {
				t.Fatalf("Marshal(%#v) = %s, want %s", tt.tag, data, tt.want)
			}
		})
	}
}

func TestTagRoundTrip(t *testing.T) {
	for _, want := range []Tag{
		{Name: "bug", Color: strPtr("#ff0000")},
		{Name: "no-color"},
	} {
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("Marshal(%#v) error = %v", want, err)
		}
		var got Tag
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal(%s) error = %v", data, err)
		}
		if got.Name != want.Name {
			t.Fatalf("round trip Name = %q, want %q", got.Name, want.Name)
		}
		if (got.Color == nil) != (want.Color == nil) || (got.Color != nil && *got.Color != *want.Color) {
			t.Fatalf("round trip Color = %v, want %v", got.Color, want.Color)
		}
	}
}

func TestTagSliceOnWorkItem(t *testing.T) {
	var story UserStory
	if err := json.Unmarshal([]byte(`{"id":1,"tags":[["bug","#ff0000"],["urgent",null]]}`), &story); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(story.Tags) != 2 {
		t.Fatalf("Tags = %#v, want 2 entries", story.Tags)
	}
	if story.Tags[0].Name != "bug" || story.Tags[0].Color == nil || *story.Tags[0].Color != "#ff0000" {
		t.Fatalf("Tags[0] = %#v", story.Tags[0])
	}
	if story.Tags[1].Name != "urgent" || story.Tags[1].Color != nil {
		t.Fatalf("Tags[1] = %#v", story.Tags[1])
	}
}

func strPtr(s string) *string { return &s }
