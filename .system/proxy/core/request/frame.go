// Quy tắt comment: chỉ giải nghĩa logic của hàm, không comment thêm gì khác.

package request

import (
	"encoding/json"
	"strings"
)

// Lõi KHÔNG tự biết chữ nào của công cụ chủ là nhiễu — người giữ hệ khai (#6). Và lõi
// luôn ĐỌC khung, chỉ CẮT khi có plugin làm chủ lời hệ thống: cắt mà không đặt gì vào
// chỗ đó là lỗ ròng.

// FrameRules — bốn cách xử chữ của công cụ chủ, khai ở won.conf.
//
// Bốn, và cố ý dừng ở bốn. Cả bốn bám vào HÌNH có sẵn của văn bản — cặp thẻ, tiêu đề
// Markdown, tiền tố một câu — nên chúng dùng được cho công cụ chủ nào cũng thế. Khung
// không có mỏ bám nào (chữ trần, không thẻ không tiêu đề) thì lõi KHÔNG cắt: mỗi ứng dụng
// nhét một kiểu vào lời hệ thống, đuổi theo được đúng một cái là tự nhận một lời hứa không
// giữ nổi. Proxy Inject chèn thứ CỦA HỆ, và với chữ của người khác thì chỉ đưa vài mỏ bám
// cơ bản nhất — không đoán (#6).
type FrameRules struct {
	// Strip — bỏ TRỌN khối, cho khối công cụ chủ nhắc chính nó (`<system-reminder>`).
	Strip []string
	// Unwrap — giữ RUỘT bỏ vỏ, vì câu của người nằm bên trong (`<userRequest>`).
	Unwrap []string
	// Sections — cắt theo TIÊU ĐỀ Markdown, tới tiêu đề cùng bậc hoặc cao hơn kế tiếp.
	// Sách của công cụ chủ không phải một khối liền: phần dạy dùng tool là tay của đệ và
	// phải giữ, phần dạy giọng điệu thì chọi với soul. Cắt cả khối là bẻ tay đệ.
	Sections []string
	// Identity — chuỗi mở đầu một lời khẳng định vai ("You are "). Gỡ đúng CÂU chứa nó.
	//
	// Cỡ chỗ cắt bằng cỡ chỗ đòi cắt: một lời khẳng định vai là một câu, đo trên 62 lời hệ thống
	// thật thì dài 12–158 ký tự, trung vị 58. Nên một lần khớp sai tốn đúng một câu, không phụ
	// thuộc công cụ chủ gói sách hướng dẫn thành mấy khối.
	Identity []string
}

// Empty — không khai gì thì không cắt gì. Rỗng là một trạng thái hợp lệ.
func (r FrameRules) Empty() bool {
	return len(r.Strip) == 0 && len(r.Unwrap) == 0 && len(r.Sections) == 0 && len(r.Identity) == 0
}

// HostFrame — khung đã đọc ra. Chỉ một câu trả lời cần cho người dùng: khung có mặt
// không. Chỗ cắt thì StripFrame tự tìm lại, vì nó cắt tại chỗ theo từng block.
type HostFrame struct {
	Present bool
	rules   FrameRules
}

// HostFrame đọc khung khỏi request. Unknown → rỗng, không đoán (#6).
func (b *Body) HostFrame(rules FrameRules) *HostFrame {
	f := &HostFrame{rules: rules}
	if b.format == FormatUnknown || rules.Empty() {
		return f
	}
	seen := func(s string) bool {
		for _, tag := range append(rules.Strip, rules.Unwrap...) {
			if _, ok := extractBlock(s, tag); ok {
				return true
			}
		}
		return false
	}
	for _, blk := range b.SystemBlocks() {
		if seen(blk) || rules.cleanSystem(blk) != strings.TrimSpace(blk) {
			f.Present = true
			return f
		}
	}
	for _, msg := range b.messages() {
		if seen(b.flatten(b.contentOf(msg))) {
			f.Present = true
			return f
		}
	}
	return f
}

// StripFrame cắt khung ĐÃ ĐỌC, tại chỗ, theo từng block. Giữ hình block là giữ đường may
// của công cụ chủ, tức bớt một bước ra khỏi đường chính sách (§ Format wire).
func (b *Body) StripFrame(f *HostFrame) bool {
	if b.format == FormatUnknown || f == nil || !f.Present {
		return false
	}
	if b.format == FormatGemini {
		changed := b.stripSystemFrameGemini(f.rules)
		if b.stripMessageFrameGemini(f.rules) {
			changed = true
		}
		return changed
	}
	if b.format == FormatOpenAIResponses {
		changed := b.stripSystemFrameResponses(f.rules)
		if b.stripMessageFrame(f.rules) {
			changed = true
		}
		return changed
	}
	changed := b.stripSystemFrame(f.rules)
	if b.stripMessageFrame(f.rules) {
		changed = true
	}
	return changed
}

// clean — một chuỗi sau khi bỏ khung. Bảng biến template cắt SAU CÙNG: bỏ khối có tag
// trước thì cái còn lại quanh câu chốt đúng là bảng, không lẫn khối của ai khác.
func (r FrameRules) clean(s string) string {
	for _, tag := range r.Unwrap {
		if inner, ok := extractBlock(s, tag); ok {
			s = inner
		}
	}
	for _, tag := range r.Strip {
		for {
			next := stripBlock(s, tag)
			if next == s {
				break
			}
			s = next
		}
	}
	return strings.TrimSpace(s)
}

// cleanSystem — như clean, thêm bỏ lời khẳng định căn cước và cắt theo mục. Chỉ dùng cho lời
// hệ thống: cả hai thứ thêm ấy là hình của sách hướng dẫn công cụ chủ, không phải hình của lời
// người (#1).
func (r FrameRules) cleanSystem(s string) string {
	s = r.dropIdentity(s)
	for _, title := range r.Sections {
		s = dropSection(s, title)
	}
	return r.clean(s)
}

// dropIdentity gỡ mọi CÂU khẳng định vai khỏi lời hệ thống, giữ lại toàn bộ phần còn lại.
// Khối chỉ có đúng câu ấy thì ra rỗng, và bên gọi bỏ khối theo luật "cắt xong rỗng thì bỏ" —
// không cần nhánh riêng cho ca đó.
func (r FrameRules) dropIdentity(s string) string {
	for _, claim := range r.Identity {
		if claim == "" {
			continue
		}
		for {
			i := claimStart(s, claim)
			if i < 0 {
				break
			}
			s = s[:i] + s[i+len(sentenceAt(s, i)):]
		}
	}
	return strings.TrimSpace(s)
}

// claimStart — vị trí claim đứng ở ĐẦU KHỐI hoặc ĐẦU MỘT DÒNG, hoặc -1. Chỉ ở hai chỗ đó một
// lời khẳng định vai mới ràng buộc; giữa câu thì nó là chữ đang bàn về vai, không phải gán vai.
// Đo trên 62 lời hệ thống thật: khớp theo đầu dòng phủ 54, khớp theo đầu khối phủ 17.
func claimStart(s, claim string) int {
	for at := 0; at < len(s); {
		i := strings.Index(s[at:], claim)
		if i < 0 {
			return -1
		}
		i += at
		if atLineStart(s, i) {
			return i
		}
		at = i + 1
	}
	return -1
}

func atLineStart(s string, i int) bool {
	for j := i - 1; j >= 0; j-- {
		if s[j] == '\n' {
			return true
		}
		if s[j] != ' ' && s[j] != '\t' && s[j] != '\r' {
			return false
		}
	}
	return true
}

// sentenceAt — câu bắt đầu tại i: tới dấu chấm câu kèm khoảng trắng, hoặc hết dòng, cái nào
// tới trước. Đọc sai ranh giới thì dừng SỚM, nên hỏng về phía gỡ thiếu chứ không gỡ lố.
func sentenceAt(s string, i int) string {
	stop := len(s)
	if nl := strings.IndexByte(s[i:], '\n'); nl >= 0 {
		stop = i + nl
	}
	for j := i; j < stop; j++ {
		if strings.IndexByte(".!?", s[j]) < 0 {
			continue
		}
		if j+1 >= stop || isSpace(rune(s[j+1])) {
			return s[i : j+1]
		}
	}
	return s[i:stop]
}

func isSpace(r rune) bool { return r == ' ' || r == '\n' || r == '\t' || r == '\r' }

// stripSystemFrame cắt khung trong lời hệ thống. Dạng chuỗi thì sửa chuỗi; dạng block
// thì sửa từng block và bỏ block rỗng ruột — hình block giữ nguyên là hình block.
func (b *Body) stripSystemFrame(rules FrameRules) bool {
	raw, ok := b.fields["system"]
	if !ok || len(raw) == 0 {
		return false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		cleaned := rules.cleanSystem(s)
		if cleaned == s {
			return false
		}
		b.ReplaceSystem(cleaned)
		return true
	}
	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return false
	}
	kept := make([]map[string]any, 0, len(blocks))
	changed := false
	for _, blk := range blocks {
		text, isText := blk["text"].(string)
		if !isText {
			kept = append(kept, blk)
			continue
		}
		// Khối mang mốc cache vẫn bị cắt, mốc ở lại trên khối đã cắt: cache khớp theo BYTE
		// gửi lên, không theo mốc, và phép cắt tất định thì tiền tố sau cắt đứng yên giữa
		// các lượt (§ Cache, "mốc của công cụ chủ và luật cắt khung"). Né khối có mốc thì
		// strip_identity/strip_sections chết hẳn — đo được: Claude Code cắm mốc 1h lên MỌI
		// khối lời dạy trong system.
		cleaned := rules.cleanSystem(text)
		if cleaned != text {
			changed = true
		}
		if cleaned == "" {
			// Khối chỉ có khung: bỏ cả khối, mốc (nếu có) đi theo. Mốc là điểm cắt tiền tố,
			// không phải nội dung — khối mang mốc đứng SAU vẫn phủ trọn tiền tố còn lại.
			continue
		}
		blk["text"] = cleaned
		kept = append(kept, blk)
	}
	if !changed {
		return false
	}
	if nb := mustJSON(kept); nb != nil {
		b.fields["system"] = nb
	}
	return true
}

// framedRole — vai lõi được phép cắt khung: **system** (đường OpenAI đặt lời hệ thống
// trong messages) và **user** — ngoại lệ duy nhất chạm lượt user (#1), và nó chỉ bỏ vỏ
// của công cụ khác. Không chạm assistant: sửa chữ model đã nói là viết lại quá khứ.
//
// MỘT định nghĩa cho cả chỗ cắt và chỗ bỏ message rỗng ruột: hai danh sách vai thì cắt
// được một vai mà không bỏ được nó, và một `content: []` là một 400.
func framedRole(role string) bool { return role == RoleUser || role == RoleSystem }

// framedRoleOf — vai lõi được phép cắt khung, rẽ theo format. Responses có item không phải
// message (function_call, function_call_output) — chúng không có content blocks để cắt,
// nên không được xét bỏ, và không được cắt.
func (b *Body) framedRoleOf(msg json.RawMessage) bool {
	if b.format == FormatOpenAIResponses {
		var m struct{ Type string }
		_ = json.Unmarshal(msg, &m)
		if m.Type != "message" {
			return false
		}
	}
	return framedRole(b.roleOfMsg(msg))
}

func (b *Body) stripMessageFrame(rules FrameRules) bool {
	msgs := b.messages()
	if msgs == nil {
		return false
	}
	changed := false
	for i, msg := range msgs {
		if !b.framedRoleOf(msg) {
			continue
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(msg, &m); err != nil {
			continue
		}
		content, ok := m["content"]
		if !ok {
			continue
		}
		// Lời hệ thống cắt như lời hệ thống, dù wire format nào chở nó: Anthropic để nó ở
		// field riêng, OpenAI để trong `messages`, mà nội dung là cùng một sách hướng dẫn.
		// `AgentTurnShaped` đã đo khối system của đường OpenAI bằng `cleanSystem` — chỗ cắt
		// dùng luật khác chỗ đo là hai luật cho một thứ, và `strip_identity`/`strip_sections`
		// thành núm xoay không có gì chuyển trên mọi công cụ đi đường OpenAI.
		next, did := cleanContent(content, rules, b.roleOfMsg(msg) == RoleSystem)
		if !did {
			continue
		}
		m["content"] = next
		if nm := mustJSON(m); nm != nil {
			msgs[i] = nm
			changed = true
		}
	}
	if !changed {
		return false
	}
	if nb := mustJSON(b.dropEmptied(msgs)); nb != nil {
		b.fields[b.messagesKey()] = nb
	}
	return true
}

// cleanContent xử content của một message, giữ đúng hình nó tới: chuỗi ra chuỗi, mảng ra
// mảng. Block không phải text (ảnh, tool_result) đi qua nguyên vẹn — lõi chỉ chạm chữ.
// `system` chọn luật của lời hệ thống, còn lại là luật của lời người.
func cleanContent(raw json.RawMessage, rules FrameRules, system bool) (json.RawMessage, bool) {
	clean := rules.clean
	if system {
		clean = rules.cleanSystem
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		cleaned := clean(s)
		if cleaned == s {
			return nil, false
		}
		return mustJSON(cleaned), true
	}
	var blocks []map[string]any
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, false
	}
	kept := make([]map[string]any, 0, len(blocks))
	changed := false
	for _, blk := range blocks {
		text, isText := blk["text"].(string)
		if !isText {
			kept = append(kept, blk)
			continue
		}
		// Cùng luật với stripSystemFrame: mốc cache trong lịch sử hội thoại (đo được:
		// messages[N].content[M]) cũng ở lại trên khối đã cắt, không cấp miễn trừ cho khối.
		cleaned := clean(text)
		if cleaned != text {
			changed = true
		}
		if cleaned == "" {
			continue
		}
		blk["text"] = cleaned
		kept = append(kept, blk)
	}
	if !changed {
		return nil, false
	}
	return mustJSON(kept), true
}

// dropEmptied bỏ message KHÔNG CÒN BLOCK NÀO sau khi cắt khung — `content: []` là thứ
// nhà cung cấp từ chối, ở bất kỳ vai nào. Xét đúng những vai `framedRole` cho phép cắt:
// vai nào ta không chạm thì ta không bỏ (#7).
//
// "Không còn block nào", KHÔNG phải "không còn chữ nào": bỏ một lượt chỉ có `tool_result`
// là để `tool_use` trơ trọi và cả request vỡ 400 (§ Format wire, bốn luật của `messages`).
func (b *Body) dropEmptied(msgs []json.RawMessage) []json.RawMessage {
	out := msgs[:0]
	for _, msg := range msgs {
		if b.framedRoleOf(msg) && contentIsEmpty(msg) {
			continue
		}
		out = append(out, msg)
	}
	return out
}

// contentIsEmpty — content của message không còn gì để gửi. Chuỗi thì xét chữ; mảng thì
// xét SỐ BLOCK, vì một block không phải text (tool_result, image, document) vẫn là nội
// dung dù không có chữ nào.
func contentIsEmpty(msg json.RawMessage) bool {
	var m struct{ Content json.RawMessage }
	if err := json.Unmarshal(msg, &m); err != nil || len(m.Content) == 0 {
		return true
	}
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return strings.TrimSpace(s) == ""
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return false // hình không đọc được thì không chạm (#6)
	}
	return len(blocks) == 0
}

// Wrap bọc text trong khối có tên — mặt VIẾT của đúng cái vỏ mà extractBlock và stripBlock
// đọc. Một nhà cho cả hai mặt: hình khối là hợp đồng trên dây, và mọi chỗ lõi đặt khối
// xuống (đất, bản đồ hệ, đóng góp plugin, vật liệu gửi model nền) phải dựng cùng một hình.
// Chép tay lần thứ hai là một bản lệch được mà bên đọc không kêu. Tag rỗng → chèn trần:
// không bịa tên khối hộ người gọi.
func Wrap(tag, text string) string {
	if tag == "" {
		return text
	}
	return "<" + tag + ">\n" + text + "\n</" + tag + ">"
}

// extractBlock — không đủ cặp tag thì coi như không tìm thấy.
func extractBlock(s, tag string) (string, bool) {
	open, end := "<"+tag+">", "</"+tag+">"
	i := strings.Index(s, open)
	if i < 0 {
		return "", false
	}
	rest := s[i+len(open):]
	j := strings.Index(rest, end)
	if j < 0 {
		return "", false
	}
	return strings.TrimSpace(rest[:j]), true
}

// stripBlock cắt trọn khối và trim whitespace hai bên vết cắt. Không đủ cặp tag → s
// nguyên vẹn.
func stripBlock(s, tag string) string {
	open, end := "<"+tag+">", "</"+tag+">"
	i := strings.Index(s, open)
	if i < 0 {
		return s
	}
	j := strings.Index(s[i+len(open):], end)
	if j < 0 {
		return s
	}
	j += i + len(open) + len(end)
	left := strings.TrimRight(s[:i], " \t\n\r")
	right := strings.TrimLeft(s[j:], " \t\n\r")
	if left != "" && right != "" {
		return left + "\n" + right
	}
	return left + right
}

// AgentTurnShaped — request có hình một lượt của đệ không: mang LỜI DẠY, hoặc mang danh
// mục tool. "Lời dạy" đo bằng chính rules đã khai, nên khối nào rules bảo là nhiễu thì
// không tính — một lời khai, hai chỗ dùng, không thêm núm.
// Cửa CHẶN, không phải cửa đoán: không chắc thì không chèn (#6).
func (b *Body) AgentTurnShaped(rules FrameRules) bool {
	for _, blk := range b.SystemBlocks() {
		// Không có nhánh riêng cho khối căn cước: `cleanSystem` đã bỏ nó, nên khối chỉ có căn
		// cước tự ra rỗng, còn khối mang sách thì còn sách và tính là CÓ lời dạy. Một định nghĩa,
		// không hai chỗ phải nhớ khớp nhau.
		if strings.TrimSpace(rules.cleanSystem(blk)) != "" {
			return true
		}
	}
	return b.hasTools()
}

// MachineReply — lời đáp request này xin về là cho MÁY đọc, không phải cho người. Cửa thứ ba,
// và là cửa duy nhất bắt được lượt việc nhà mang ĐỦ lời dạy VÀ đủ hội thoại: một lượt của đệ xin
// về chữ cho người đọc, một lượt việc nhà xin về đúng một object khớp schema.
//
// Dấu hiệu nằm trong chính schema của nhà cung cấp, nên đây là đọc HÌNH, không có danh sách chuỗi
// nào phải nuôi. Anthropic không có field cho việc này (nó ép cấu trúc bằng `tool_choice`) nên
// đường ấy trả false: không đoán thay họ (#6).
func (b *Body) MachineReply() bool {
	switch b.format {
	case FormatGemini:
		var cfg struct {
			ResponseMimeType   string          `json:"responseMimeType"`
			ResponseSchema     json.RawMessage `json:"responseSchema"`
			ResponseJSONSchema json.RawMessage `json:"responseJsonSchema"`
		}
		if json.Unmarshal(b.fields["generationConfig"], &cfg) != nil {
			return false
		}
		return len(cfg.ResponseSchema) > 0 || len(cfg.ResponseJSONSchema) > 0 ||
			(cfg.ResponseMimeType != "" && cfg.ResponseMimeType != "text/plain")
	case FormatOpenAI:
		var rf struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(b.fields["response_format"], &rf) != nil {
			return false
		}
		return rf.Type == "json_schema" || rf.Type == "json_object"
	case FormatOpenAIResponses:
		var tf struct {
			Format struct {
				Type string `json:"type"`
			} `json:"format"`
		}
		if json.Unmarshal(b.fields["text"], &tf) != nil {
			return false
		}
		return tf.Format.Type == "json_schema" || tf.Format.Type == "json_object"
	}
	return false
}

// ConversationShaped — request này có hình một HỘI THOẠI không: có lịch sử để tiếp, hoặc
// có tay để dùng. Cửa thứ hai, hỏi câu khác `AgentTurnShaped`: cái kia hỏi "có lời dạy
// không". Một message và không tools là một câu hỏi rời — cái giá, và đánh đổi với client
// không gửi tools: § Format wire, "Lượt việc nhà".
func (b *Body) ConversationShaped() bool {
	return b.conversationMessages() > 1 || b.hasTools()
}

// conversationMessages — số message KHÔNG phải lời hệ thống. Đếm thế để đường OpenAI
// (lời hệ thống nằm trong `messages`) và đường Anthropic (field `system` riêng) cho
// cùng một con số (#7).
func (b *Body) conversationMessages() int {
	n := 0
	for _, msg := range b.messages() {
		if b.roleOfMsg(msg) != RoleSystem {
			n++
		}
	}
	return n
}

// hasTools — có danh mục tool và trong đó có món nào. Field rỗng không tính: có field
// mà không có món nào thì tay vẫn trắng.
func (b *Body) hasTools() bool {
	raw, ok := b.fields["tools"]
	if !ok {
		return false
	}
	var tools []json.RawMessage
	if err := json.Unmarshal(raw, &tools); err != nil {
		return false
	}
	// Gemini lồng món xuống một tầng, nên ĐẾM PHẦN TỬ ở đây là đếm cái vỏ. Đo được: Gemini CLI
	// gửi `tools: [{"name": ""}]` — một nhóm, không món nào. Đếm vỏ thì tay trắng hoá tay đầy,
	// mà đó đúng là điều câu trên cấm. Cùng một nguyên tắc, đếm ở đúng tầng của nó.
	if b.format == FormatGemini {
		for _, t := range tools {
			if geminiGroupHasTool(t) {
				return true
			}
		}
		return false
	}
	return len(tools) > 0
}

// dropSection cắt từ dòng tiêu đề tới tiêu đề cùng bậc hoặc cao hơn kế tiếp; mục con đi
// theo. Khớp theo TIỀN TỐ tên, không phân biệt hoa thường, bỏ qua số bậc `#` — người khai
// chỉ biết cái tên mình đọc thấy: thật có `# Text output (does not apply to tool calls)`,
// và khai "Text output" phải trúng.
func dropSection(s, title string) string {
	lines := strings.Split(s, "\n")
	want := strings.ToLower(strings.TrimSpace(title))
	out := make([]string, 0, len(lines))
	cutLevel := 0 // 0 = đang không cắt
	for _, line := range lines {
		level, name := heading(line)
		if cutLevel > 0 {
			// Thoát mục khi gặp tiêu đề cùng bậc hoặc cao hơn (số `#` nhỏ hơn/bằng).
			if level > 0 && level <= cutLevel {
				cutLevel = 0
			} else {
				continue
			}
		}
		if level > 0 && strings.HasPrefix(strings.ToLower(name), want) {
			cutLevel = level
			continue
		}
		out = append(out, line)
	}
	if cutLevel == 0 && len(out) == len(lines) {
		return s // không thấy mục nào: trả nguyên, không chạm
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// heading — dòng này là tiêu đề Markdown bậc mấy, tên gì. Không phải tiêu đề → 0.
func heading(line string) (int, string) {
	t := strings.TrimLeft(line, " \t")
	n := 0
	for n < len(t) && t[n] == '#' {
		n++
	}
	if n == 0 || n > 6 || n >= len(t) || (t[n] != ' ' && t[n] != '\t') {
		return 0, ""
	}
	return n, strings.TrimSpace(t[n:])
}
