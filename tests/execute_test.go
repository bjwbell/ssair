package tests

import "testing"

func TestT0(t *testing.T) {
	t.Log("TEST COUNT")
	t0()

}

func TestT1(t *testing.T) {
	t.Log("TEST COUNT")
	t.Log("t1: ", t1())
}
