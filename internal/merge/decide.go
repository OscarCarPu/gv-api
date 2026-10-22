package merge

type Choice string

const (
	Home Choice = "home"
	EC2  Choice = "ec2"
)

type Do int

const (
	Nothing Do = iota
	DoInsert
	DoUpdate
	DoDelete
	Ask
)

// decide turns one change made on home into what to do on ec2. ec2 is nil when the row is absent there.
func decide(c Change, ec2 *Row, choice Choice) (Do, string) {
	switch {
	case c.Op == Insert && ec2 == nil:
		return DoInsert, ""
	case same(ec2, c.New):
		return Nothing, ""
	case c.Op == Update && ec2 == nil:
		return pick(choice, DoInsert, "edited on home, deleted on ec2")
	case c.Op != Insert && same(ec2, c.Old):
		return apply(c.Op), ""
	}

	switch c.Op {
	case Insert:
		return pick(choice, DoUpdate, "created on both sides with the same key")
	case Update:
		return pick(choice, DoUpdate, "same row edited on both sides")
	default:
		return pick(choice, DoDelete, "deleted on home, edited on ec2")
	}
}

// pick resolves a clash: home's choice does what home did, ec2's keeps the row as it is.
func pick(choice Choice, homeWins Do, reason string) (Do, string) {
	switch choice {
	case Home:
		return homeWins, reason
	case EC2:
		return Nothing, reason
	}
	return Ask, reason
}

func apply(op Op) Do {
	switch op {
	case Insert:
		return DoInsert
	case Update:
		return DoUpdate
	}
	return DoDelete
}
