package hal

import (
	"encoding/json"
)

//
// Error
//

type Error struct {
	ErrorIdentifier string `json:"errorIdentifier"`
	Message         string `json:"message"`
}

func (res *Error) ResourceType() string {
	return "Error"
}

func (res *Error) IsError() *Error {
	return res
}

func (res *Error) Decode(mData map[string]json.RawMessage) error {
	for key, val := range mData {
		switch key {
		case "errorIdentifier":
			if err := json.Unmarshal(val, &res.ErrorIdentifier); err != nil {
				return err
			}
		case "message":
			if err := json.Unmarshal(val, &res.Message); err != nil {
				return err
			}
		default:
		}
	}
	return nil
}

func NewError() *Error {
	return &Error{}
}

// Register Resource Factories
func init() {
	resourceTypes["Error"] = func() Resource {
		return NewError()
	}
}
