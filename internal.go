package cache

import "errors"

// Jsoner
type Jsoner interface {
	MarshalJSON() ([]byte, error)
	UnmarshalJSON([]byte) error
}

// Binarier
type Binarier interface {
	MarshalBinary() ([]byte, error)
	UnmarshalBinary([]byte) error
}

// Null
type Null struct{}

func (n Null) MarshalBinary() ([]byte, error) {
	return []byte{}, nil
}

func (n Null) UnmarshalBinary(b []byte) error {
	if len(b) > 0 {
		return errors.New("bytes not null")
	}
	return nil
}

// nullJsoner
type nullJsoner struct{}

func (n nullJsoner) MarshalJSON() ([]byte, error) {
	return []byte{}, nil
}

func (n nullJsoner) UnmarshalJSON(b []byte) error {
	if len(b) > 0 {
		return errors.New("bytes not null")
	}
	return nil
}
