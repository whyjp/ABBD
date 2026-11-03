package main

import "github.com/whyjp/abbd/internal/model"

// flow stores a flow from a draft, or prints the stored one, as figure does
// for every kind.
func (e env) flow(key string, args []string) error { return figure[model.Step](e, key, args) }
