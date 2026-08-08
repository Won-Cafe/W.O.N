// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package debug

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// orderedMap giữ cặp khoá-giá trị đúng THỨ TỰ xuất hiện trong JSON gốc. Nhật
// ký chẩn bệnh là để SO byte chủ gửi với byte ta gửi đi — map chuẩn của Go
// marshal theo bảng chữ cái, và xáo thứ tự đúng chỗ cần so là tự làm mù mắt
// người đọc. Chỉ dùng khi cần SỬA một giá trị lồng sâu mà vẫn giữ nguyên vị
// trí và thứ tự của mọi khoá khác xung quanh nó.
type orderedMap struct {
	keys   []string
	values map[string]json.RawMessage
}

func (m *orderedMap) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("orderedMap: expected object, got %v", tok)
	}
	m.values = map[string]json.RawMessage{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("orderedMap: expected string key, got %v", keyTok)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return err
		}
		if _, dup := m.values[key]; !dup {
			m.keys = append(m.keys, key)
		}
		m.values[key] = raw
	}
	_, err = dec.Token() // '}'
	return err
}

func (m *orderedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(m.values[k])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func (m *orderedMap) get(key string) (json.RawMessage, bool) {
	v, ok := m.values[key]
	return v, ok
}

func (m *orderedMap) set(key string, v json.RawMessage) {
	if _, ok := m.values[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.values[key] = v
}
