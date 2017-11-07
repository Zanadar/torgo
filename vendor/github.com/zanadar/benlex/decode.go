package benlex

import (
	"bufio"
	"errors"
	"io"
	"strconv"
)

type decoder struct {
	bufio.Reader
}

func (decoder *decoder) readIntUntil(until byte) (interface{}, error) {
	res, err := decoder.ReadSlice(until)
	if err != nil {
		return nil, err
	}
	str := string(res[:len(res)-1])
	value, err := strconv.ParseInt(str, 10, 64)

	return value, err
}

func (decoder *decoder) readInt() (interface{}, error) {
	return decoder.readIntUntil('e')
}

func (decoder *decoder) readList() ([]interface{}, error) {
	var list []interface{}
	for {
		ch, err := decoder.ReadByte()
		if err != nil {
			return nil, err
		}
		if ch == 'e' {
			break
		}
		item, err := decoder.ReadInterfaceType(ch)
		list = append(list, item)
	}
	return list, nil
}

func (decoder *decoder) ReadInterfaceType(identifier byte) (item interface{}, err error) {
	switch identifier {
	case 'l':
		item, err = decoder.readList()
	case 'i':
		item, err = decoder.readInt()
	case 'd':
		item, err = decoder.readDictionary()
	default:
		if err := decoder.UnreadByte(); err != nil {
			return nil, err
		}
		item, err = decoder.readString()
	}

	return item, err
}

func (decoder *decoder) readString() (string, error) {
	len, err := decoder.readIntUntil(':')
	if err != nil {
		return "", err
	}

	stringLength, ok := len.(int64)
	if !ok {
		return "", errors.New("Bad string length")
	}

	if stringLength < 0 {
		return "", errors.New("String is too short")
	}
	buffer := make([]byte, len.(int64))
	_, err = io.ReadFull(decoder, buffer)
	return string(buffer), err
}

func (decoder *decoder) readDictionary() (map[string]interface{}, error) {
	dict := make(map[string]interface{})

	for {
		key, err := decoder.readString()
		if err != nil {
			return nil, err
		}

		ch, err := decoder.ReadByte()
		item, err := decoder.ReadInterfaceType(ch)
		dict[key] = item
		nextItem, err := decoder.ReadByte()
		if nextItem == 'e' {
			break
		} else if err := decoder.UnreadByte(); err != nil {
			return nil, err
		}
	}
	return dict, nil
}

func Decode(reader io.Reader) (map[string]interface{}, error) {
	decoder := decoder{*bufio.NewReader(reader)}
	if firstByte, err := decoder.ReadByte(); err != nil {
		return make(map[string]interface{}), nil
	} else if firstByte != 'd' {
		return nil, errors.New("Bendcode must be wrapped in a dict")
	}
	return decoder.readDictionary()
}
