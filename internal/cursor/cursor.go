package cursor

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

type Keyset struct {
	Time time.Time `json:"time"`
	ID   string    `json:"id"`
}

func Encode(value Keyset) (string, error) {
	if value.ID == "" {
		return "", errors.New("cursor id is required")
	}
	if value.Time.IsZero() {
		return "", errors.New("cursor time is required")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func Decode(value string) (Keyset, error) {
	if value == "" {
		return Keyset{}, errors.New("cursor is required")
	}
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return Keyset{}, errors.New("cursor is not valid base64url")
	}
	var decoded Keyset
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return Keyset{}, errors.New("cursor payload is invalid")
	}
	if decoded.ID == "" || decoded.Time.IsZero() {
		return Keyset{}, errors.New("cursor payload is incomplete")
	}
	return decoded, nil
}
