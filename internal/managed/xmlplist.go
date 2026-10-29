package managed

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// parseXML decodes an XML property list. Only the elements a profile uses are accepted, and nesting is capped.
// encoding/xml does not fetch external DTDs or expand custom entities, so a DOCTYPE cannot pull in other data.
func parseXML(data []byte) (any, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))

	root, err := nextElement(dec)
	if err != nil {
		return nil, fmt.Errorf("plist: not an XML plist: %w", err)
	}
	if root.Name.Local != "plist" {
		return nil, errors.New("plist: root element is not <plist>")
	}
	valueStart, err := nextElement(dec)
	if err != nil {
		return nil, err
	}
	if valueStart.Name.Local == "plist" {
		return nil, errors.New("plist: no value inside <plist>")
	}
	v, err := xmlValue(dec, valueStart, 0)
	if err != nil {
		return nil, err
	}
	// Only the end of <plist> may follow the value.
	end, err := nextEnd(dec)
	if err != nil {
		return nil, err
	}
	if end.Name.Local != "plist" {
		return nil, fmt.Errorf("plist: unexpected </%s>", end.Name.Local)
	}
	return v, nil
}

// xmlValue decodes the value whose start element has been read. Its end element is consumed here.
func xmlValue(dec *xml.Decoder, start xml.StartElement, depth int) (any, error) {
	if depth > maxNestingDepth {
		return nil, errTooDeep
	}
	name := start.Name.Local
	switch name {
	case "dict":
		return xmlDict(dec, depth)
	case "array":
		return xmlArray(dec, depth)
	case "true", "false":
		if _, err := xmlText(dec, name); err != nil {
			return nil, err
		}
		return name == "true", nil
	case "date", "data":
		if _, err := xmlText(dec, name); err != nil {
			return nil, err
		}
		return opaque{name}, nil
	case "string":
		return xmlText(dec, name)
	case "integer":
		text, err := xmlText(dec, name)
		if err != nil {
			return nil, err
		}
		n, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("plist: bad integer %q: %w", text, err)
		}
		return n, nil
	case "real":
		text, err := xmlText(dec, name)
		if err != nil {
			return nil, err
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil {
			return nil, fmt.Errorf("plist: bad real %q: %w", text, err)
		}
		return f, nil
	}
	return nil, fmt.Errorf("plist: unsupported element <%s>", name)
}

// xmlDict reads key and value pairs up to </dict>. The opening <dict> has already been read.
func xmlDict(dec *xml.Decoder, depth int) (any, error) {
	dict := map[string]any{}
	for {
		tok, err := nextToken(dec)
		if err != nil {
			return nil, err
		}
		if end, isEnd := tok.(xml.EndElement); isEnd {
			if end.Name.Local != "dict" {
				return nil, fmt.Errorf("plist: unexpected </%s> in dictionary", end.Name.Local)
			}
			return dict, nil
		}
		keyStart, ok := tok.(xml.StartElement)
		if !ok || keyStart.Name.Local != "key" {
			return nil, errors.New("plist: expected <key> in dictionary")
		}
		key, err := xmlText(dec, "key")
		if err != nil {
			return nil, err
		}
		valueStart, err := nextElement(dec)
		if err != nil {
			return nil, err
		}
		v, err := xmlValue(dec, valueStart, depth+1)
		if err != nil {
			return nil, err
		}
		dict[key] = v
	}
}

// xmlArray reads values up to </array>. The opening <array> has already been read.
func xmlArray(dec *xml.Decoder, depth int) (any, error) {
	arr := []any{}
	for {
		tok, err := nextToken(dec)
		if err != nil {
			return nil, err
		}
		if end, isEnd := tok.(xml.EndElement); isEnd {
			if end.Name.Local != "array" {
				return nil, fmt.Errorf("plist: unexpected </%s> in array", end.Name.Local)
			}
			return arr, nil
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			return nil, errors.New("plist: expected a value element in array")
		}
		v, err := xmlValue(dec, start, depth+1)
		if err != nil {
			return nil, err
		}
		arr = append(arr, v)
	}
}

// xmlText returns the text of element name, whose start tag has been read, and consumes its end tag. Child elements
// are an error.
func xmlText(dec *xml.Decoder, name string) (string, error) {
	var b strings.Builder
	for {
		tok, err := dec.Token() // raw: this is where non-whitespace text is allowed
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			if t.Name.Local != name {
				return "", fmt.Errorf("plist: mismatched </%s> inside <%s>", t.Name.Local, name)
			}
			return b.String(), nil
		case xml.StartElement:
			return "", fmt.Errorf("plist: unexpected <%s> inside <%s>", t.Name.Local, name)
		}
	}
}

// nextToken returns the next token that matters. Comments, processing instructions and directives are skipped, and
// so is whitespace-only text. Any other text is an error, because a plist has no text between its elements.
func nextToken(dec *xml.Decoder) (xml.Token, error) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement, xml.EndElement:
			return t, nil
		case xml.CharData:
			if len(bytes.TrimSpace(t)) != 0 {
				return nil, errors.New("plist: unexpected text between elements")
			}
		}
	}
}

// nextElement returns the next start element.
func nextElement(dec *xml.Decoder) (xml.StartElement, error) {
	tok, err := nextToken(dec)
	if err != nil {
		return xml.StartElement{}, err
	}
	start, ok := tok.(xml.StartElement)
	if !ok {
		return xml.StartElement{}, errors.New("plist: expected an element")
	}
	return start, nil
}

// nextEnd returns the next end element.
func nextEnd(dec *xml.Decoder) (xml.EndElement, error) {
	tok, err := nextToken(dec)
	if err != nil {
		return xml.EndElement{}, err
	}
	end, ok := tok.(xml.EndElement)
	if !ok {
		return xml.EndElement{}, errors.New("plist: unexpected content after the value")
	}
	return end, nil
}
