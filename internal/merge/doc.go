// Package merge three-way merges the home database into the one restored from EC2.
//
// The base is the dump both sides started from, home is what it holds now, and ec2 is the target.
// Every row home changed since the base is applied to ec2 unless ec2 changed the same row, in
// which case the caller decides. Tables, keys and order come from the catalog; EC2's sequences
// start far above the base's, so ids created on each side never clash.
package merge
