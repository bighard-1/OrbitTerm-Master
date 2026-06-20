package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func validVectorClock(raw string) bool {
	if strings.TrimSpace(raw) == "" || !json.Valid([]byte(raw)) {
		return false
	}
	_, err := parseVectorClockJSON(raw)
	return err == nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, char := range value {
		switch index {
		case 8, 13, 18, 23:
			if char != '-' {
				return false
			}
		default:
			if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
				return false
			}
		}
	}
	return true
}

func validHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func mergeVectorClocks(leftJSON, rightJSON string) (string, error) {
	left, err := parseVectorClockJSON(leftJSON)
	if err != nil {
		return "", err
	}
	right, err := parseVectorClockJSON(rightJSON)
	if err != nil {
		return "", err
	}
	for key, value := range right {
		if value > left[key] {
			left[key] = value
		}
	}
	encoded, err := json.Marshal(left)
	if err != nil {
		return "", fmt.Errorf("marshal vector clock: %w", err)
	}
	return string(encoded), nil
}

type vectorClockRelation int

const (
	vectorClockEqual vectorClockRelation = iota
	vectorClockNewer
	vectorClockOlder
	vectorClockConflict
)

func compareVectorClock(incomingJSON, currentJSON string) (vectorClockRelation, error) {
	incoming, err := parseVectorClockJSON(incomingJSON)
	if err != nil {
		return vectorClockConflict, err
	}
	current, err := parseVectorClockJSON(currentJSON)
	if err != nil {
		return vectorClockConflict, err
	}

	allKeys := make(map[string]struct{}, len(incoming)+len(current))
	for key := range incoming {
		allKeys[key] = struct{}{}
	}
	for key := range current {
		allKeys[key] = struct{}{}
	}

	incomingGreater := false
	currentGreater := false
	for key := range allKeys {
		if incoming[key] > current[key] {
			incomingGreater = true
		}
		if incoming[key] < current[key] {
			currentGreater = true
		}
	}

	switch {
	case !incomingGreater && !currentGreater:
		return vectorClockEqual, nil
	case incomingGreater && !currentGreater:
		return vectorClockNewer, nil
	case !incomingGreater && currentGreater:
		return vectorClockOlder, nil
	default:
		return vectorClockConflict, nil
	}
}

func parseVectorClockJSON(raw string) (map[string]int64, error) {
	parsed := make(map[string]int64)
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}
	for key, value := range parsed {
		if strings.TrimSpace(key) == "" || value < 0 {
			return nil, errors.New("invalid vector clock entry")
		}
	}
	return parsed, nil
}
