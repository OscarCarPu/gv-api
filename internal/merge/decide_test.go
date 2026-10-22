package merge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func row(sig string) *Row { return &Row{Key: "1", sig: sig} }

func TestDecide(t *testing.T) {
	base, home, other := row("base"), row("home"), row("other")

	tests := []struct {
		name   string
		change Change
		ec2    *Row
		choice Choice
		do     Do
		reason string
	}{
		{"insert with nothing on ec2", Change{Op: Insert, New: home}, nil, "", DoInsert, ""},
		{"insert already identical on ec2", Change{Op: Insert, New: home}, row("home"), "", Nothing, ""},
		{"insert clashes", Change{Op: Insert, New: home}, other, "", Ask, "created on both sides with the same key"},
		{"insert clash, home wins", Change{Op: Insert, New: home}, other, Home, DoUpdate, "created on both sides with the same key"},
		{"insert clash, ec2 wins", Change{Op: Insert, New: home}, other, EC2, Nothing, "created on both sides with the same key"},

		{"update of a row ec2 left alone", Change{Op: Update, Old: base, New: home}, row("base"), "", DoUpdate, ""},
		{"update ec2 already has", Change{Op: Update, Old: base, New: home}, row("home"), "", Nothing, ""},
		{"update edited on both sides", Change{Op: Update, Old: base, New: home}, other, "", Ask, "same row edited on both sides"},
		{"update edited on both, home wins", Change{Op: Update, Old: base, New: home}, other, Home, DoUpdate, "same row edited on both sides"},
		{"update edited on both, ec2 wins", Change{Op: Update, Old: base, New: home}, other, EC2, Nothing, "same row edited on both sides"},
		{"update of a row ec2 deleted", Change{Op: Update, Old: base, New: home}, nil, "", Ask, "edited on home, deleted on ec2"},
		{"update of a deleted row, home wins", Change{Op: Update, Old: base, New: home}, nil, Home, DoInsert, "edited on home, deleted on ec2"},
		{"update of a deleted row, ec2 wins", Change{Op: Update, Old: base, New: home}, nil, EC2, Nothing, "edited on home, deleted on ec2"},

		{"delete of a row ec2 left alone", Change{Op: Delete, Old: base}, row("base"), "", DoDelete, ""},
		{"delete of a row ec2 deleted", Change{Op: Delete, Old: base}, nil, "", Nothing, ""},
		{"delete of a row ec2 edited", Change{Op: Delete, Old: base}, other, "", Ask, "deleted on home, edited on ec2"},
		{"delete of an edited row, home wins", Change{Op: Delete, Old: base}, other, Home, DoDelete, "deleted on home, edited on ec2"},
		{"delete of an edited row, ec2 wins", Change{Op: Delete, Old: base}, other, EC2, Nothing, "deleted on home, edited on ec2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			do, reason := decide(tt.change, tt.ec2, tt.choice)
			assert.Equal(t, tt.do, do)
			assert.Equal(t, tt.reason, reason)
		})
	}
}

func TestChanges(t *testing.T) {
	r := func(key, sig string) Row { return Row{Key: key, sig: sig} }
	base := map[string]Row{"1": r("1", "a"), "2": r("2", "b"), "3": r("3", "c")}
	side := map[string]Row{"1": r("1", "a"), "2": r("2", "B"), "4": r("4", "d")}

	got := changes("t", base, side)

	ops := map[string]Op{}
	for _, c := range got {
		ops[c.Key] = c.Op
	}
	assert.Equal(t, map[string]Op{"2": Update, "3": Delete, "4": Insert}, ops)
}

func TestOrderPutsParentsFirst(t *testing.T) {
	byName := map[string]*Table{
		"todos":    {Name: "todos", Parents: []string{"tasks"}},
		"tasks":    {Name: "tasks", Parents: []string{"projects"}},
		"projects": {Name: "projects", Parents: []string{"projects"}},
		"habits":   {Name: "habits"},
	}
	got, err := order(byName, []string{"todos", "tasks", "projects", "habits"})

	assert.NoError(t, err)
	var names []string
	for _, tb := range got {
		names = append(names, tb.Name)
	}
	assert.Equal(t, []string{"habits", "projects", "tasks", "todos"}, names)
}

func TestOrderRejectsCycles(t *testing.T) {
	byName := map[string]*Table{
		"a": {Name: "a", Parents: []string{"b"}},
		"b": {Name: "b", Parents: []string{"a"}},
	}
	_, err := order(byName, []string{"a", "b"})
	assert.ErrorContains(t, err, "cycle")
}
