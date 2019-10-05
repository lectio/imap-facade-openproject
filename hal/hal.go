package hal

import (
	"encoding/json"
	"fmt"
	"io"
)

var (
	resourceTypes = make(map[string]ResourceFactory)
)

type ResourceFactory = func() Resource

func NewResource(typeName string) Resource {
	factory, ok := resourceTypes[typeName]
	if ok {
		return factory()
	}
	// No specialized type, use generic ResourceObject
	return &ResourceObject{
		Type: typeName,
	}
}

type Link struct {
	Href  string `json:"href"`
	Title string `json:"title"`
	// TODO: add other fields
}

type Resource interface {
	ResourceType() string
	Decode(map[string]json.RawMessage) error
	IsError() *Error
}

//
// Generic Resource Object
//

type ResourceObject struct {
	Type     string                `json:"_type"`
	Links    map[string]Link       `json:"_links"`
	Embedded map[string][]Resource `json:"_embedded"`
	Fields   map[string]interface{}
}

func (res *ResourceObject) getString(field string) string {
	if val, ok := res.Fields[field]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func (res *ResourceObject) getInt(field string) int {
	if val, ok := res.Fields[field]; ok {
		if n, ok := val.(int); ok {
			return n
		}
	}
	return 0
}

func (res *ResourceObject) GetLink(name string) *Link {
	if link, ok := res.Links[name]; ok {
		return &link
	}
	return nil
}

func (res *ResourceObject) ResourceType() string {
	return res.Type
}

func (res *ResourceObject) IsError() *Error {
	return nil
}

func (res *ResourceObject) Decode(mData map[string]json.RawMessage) error {
	for key, val := range mData {
		switch key {
		case "_type":
		case "_links":
			if err := json.Unmarshal(val, &res.Links); err != nil {
				return err
			}
		case "_embedded":
			// Unmarshal map of arrays of RawMessages
			var rawEmbedded map[string][]json.RawMessage
			if err := json.Unmarshal(val, &rawEmbedded); err != nil {
				return err
			}
			// Unmarshal each embedded resource
			res.Embedded = make(map[string][]Resource)
			for eKey, eVals := range rawEmbedded {
				arr := make([]Resource, 0, len(eVals))
				for _, eVal := range eVals {
					if subRes, err := Unmarshal(eVal); err != nil {
						return err
					} else {
						arr = append(arr, subRes)
					}
				}
				res.Embedded[eKey] = arr
			}
		default:
			var field interface{}
			if err := json.Unmarshal(val, &field); err != nil {
				return err
			}
			if res.Fields == nil {
				res.Fields = make(map[string]interface{})
			}
			res.Fields[key] = field
		}
	}
	return nil
}

func decodeResource(mData map[string]json.RawMessage) (Resource, error) {
	var res Resource

	// Decode resource type
	if typeRaw, ok := mData["_type"]; ok {
		var typeName string
		if err := json.Unmarshal(typeRaw, &typeName); err != nil {
			return nil, err
		}
		res = NewResource(typeName)
	} else {
		return nil, fmt.Errorf("Missing '_type' field, unknown resource type.")
	}

	if err := res.Decode(mData); err != nil {
		return nil, err
	}
	return res, nil
}

//
// Unmarshal and detect resource type
//
func Unmarshal(data []byte) (Resource, error) {
	// decode json
	var mData map[string]json.RawMessage
	if err := json.Unmarshal(data, &mData); err != nil {
		return nil, err
	}

	return decodeResource(mData)
}

//
// Decode Resource from stream
//
func Decode(r io.Reader) (Resource, error) {
	dec := json.NewDecoder(r)
	// decode json
	var mData map[string]json.RawMessage
	if err := dec.Decode(&mData); err != nil {
		return nil, err
	}

	return decodeResource(mData)
}
