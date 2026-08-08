// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package debug

import (
	"bytes"
	"encoding/json"

	"won/proxy/core/plugin"
)

// Hai file thân soi CẤU TRÚC, không soi nội dung: giữ trọn hình của request — mọi khoá,
// đúng thứ tự gốc, kể cả schema tool — còn mọi chuỗi thì cắt về hai mép. Cắt ĐỀU, không
// phân biệt chữ của ai: một luật cho lời người, khối của lõi, blob mã hoá và mô tả tool.
// Nội dung đọc được sống ở `asked`/`replied` của debug_detail.json, chỗ không có trần.
//
// Cắt chạy trong Log.Write, tức SAU rp.ServeHTTP: thân đã ra khỏi cửa trước khi nhật ký
// chạm tới, nên một lỗi ở đây không đổi được byte đi upstream. Thêm một lớp: cắt dựng
// byte MỚI, không ghi qua bản gốc — có test khoá.
const (
	cutThreshold = 100 // ký tự — ngắn hơn ngưỡng thì cắt chỉ mất chữ mà không bớt được gì
	cutEdge      = 25  // ký tự mỗi mép — giữ CẢ hai đầu: một khối hỏng thường hỏng ở mép cuối
	cutMark      = " … "
)

// cutBody cắt mọi chuỗi trong thân. Không phải JSON đọc được (nén, hỏng) → trả nguyên
// bản, đừng đoán nó đúng hình (#6).
func cutBody(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}
	out, ok := cutValue(json.RawMessage(raw))
	if !ok {
		return raw
	}
	return out
}

// cutValue rẽ theo HÌNH của giá trị, không theo tên khoá: object và mảng đi tiếp xuống,
// chuỗi bị cắt, số/bool/null giữ nguyên. Object đi qua orderedMap nên thứ tự khoá gốc
// còn nguyên — đó là cái file này tồn tại để cho thấy.
//
// Dựng lại bằng cách NỐI BYTE, không bằng json.Marshal: Marshal escape HTML ở mọi tầng nó
// đi qua, kể cả khi tầng dưới đã trả byte đúng — và `<W.O.N>` hoá `<W.O.N>` thì
// tên thẻ không còn đọc được bằng mắt.
func cutValue(raw json.RawMessage) (json.RawMessage, bool) {
	switch firstToken(raw) {
	case '{':
		var m orderedMap
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, false
		}
		for _, k := range m.keys {
			v, _ := m.get(k)
			cut, ok := cutValue(v)
			if !ok {
				return nil, false
			}
			m.set(k, cut)
		}
		b, err := m.MarshalJSON()
		if err != nil {
			return nil, false
		}
		return b, true
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, false
		}
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, item := range items {
			cut, ok := cutValue(item)
			if !ok {
				return nil, false
			}
			if i > 0 {
				buf.WriteByte(',')
			}
			buf.Write(cut)
		}
		buf.WriteByte(']')
		return buf.Bytes(), true
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, false
		}
		return marshalText(cutText(s))
	}
	return raw, true
}

// firstToken — ký tự mở của một giá trị JSON, bỏ khoảng trắng đứng trước. 0 nghĩa là rỗng.
func firstToken(raw json.RawMessage) byte {
	for _, c := range raw {
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		}
		return c
	}
	return 0
}

// cutCallSystem cắt lời hệ thống của từng lượt gọi model nền về hai mép, cùng trần với
// hai file thân. Lời ấy là TRỌN soul file của plugin, một bản đã nằm trên đĩa: in lại đủ
// chỉ lấp nhật ký — cùng luật với khối bản sắc ở Gather ("bản sắc chỉ đếm"). Hai mép còn
// đủ để nhận ra prompt nào đã đi, và `user` thì giữ nguyên vì đó là chữ của lượt này.
//
// Dựng slice MỚI ở mọi tầng chạm tới: `Stage.Plugins` là của Collector, còn `Trace.Calls`
// là của lượt gọi mà cửa chạy khô (control/trigger.go) đọc nguyên văn. Cắt tại chỗ ở đây
// là cắt luôn cái bên ấy đang trả về.
func cutCallSystem(stages []Stage) []Stage {
	out := make([]Stage, len(stages))
	copy(out, stages)
	for i, st := range out {
		if len(st.Plugins) == 0 {
			continue
		}
		plugins := make([]plugin.PluginDetail, len(st.Plugins))
		copy(plugins, st.Plugins)
		for j, p := range plugins {
			if len(p.Calls) == 0 {
				continue
			}
			calls := make([]plugin.Call, len(p.Calls))
			copy(calls, p.Calls)
			for k := range calls {
				calls[k].System = cutText(calls[k].System)
			}
			plugins[j].Calls = calls
		}
		out[i].Plugins = plugins
	}
	return out
}

// cutText — hai mép và dấu ở giữa. Cắt theo rune: cắt theo byte thì một chữ tiếng Việt
// bị chặt đôi ngay tại mép.
func cutText(s string) string {
	r := []rune(s)
	if len(r) <= cutThreshold {
		return s
	}
	return string(r[:cutEdge]) + cutMark + string(r[len(r)-cutEdge:])
}

// marshalText — chuỗi thành literal JSON KHÔNG escape HTML: `<W.O.N>` phải ra đúng dấu
// ngoặc, mà json.Marshal thì tự đổi nó thành <.
func marshalText(s string) (json.RawMessage, bool) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return nil, false
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), true
}
