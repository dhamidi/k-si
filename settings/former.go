package settings

// ToFormer is implemented by a setting's Go type to define its form — the one
// shape that drives both render and parse. The DEFAULT former (FormOf, derive.go)
// derives only the OBVIOUS kinds by reflection; a type IMPLEMENTS ToForm
// explicitly to opt into a richer kind (choice, secret, file, group) or a shape
// that changes as it is filled (an Update). There is no safe way to guess from a
// Go type that a string "is a secret" — guessing wrong leaks it — so the richer
// kinds are always an explicit, one-method opt-in.
type ToFormer interface {
	ToForm() Form
}
