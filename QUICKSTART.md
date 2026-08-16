# Bắt đầu

Chào bạn. Tờ này chỉ để bạn chạy được phần máy của W.O.N, không hơn. Bạn không cần biết lập trình, nhưng cần biết chút về terminal: gõ vài dòng lệnh, sửa một file text.

Muốn biết W.O.N *là gì* thì đọc [README.md](README.md), dài hơn nhưng đó mới là phần đáng đọc.

---

## W.O.N chạy được mà không cần máy

Nói trước điều này cho đỡ ngợp: ba trụ, phép thử và sáu cách lấn nằm trọn trên giấy. Một cuốn sổ là đủ để bắt đầu.

Phần còn lại của tờ này là tuỳ chọn: một proxy nhỏ ngồi giữa agent AI bạn đang dùng và nhà cung cấp model, để hệ tự đi theo bạn mỗi lượt. Bạn không phải nhắc lại mình là ai, đang làm gì.

Không chạy nó, W.O.N vẫn là W.O.N.

---

## 1. Cài Go

Một lần duy nhất. Chưa có bản dựng sẵn để tải về nhấp chạy, nên bước này là cài Go — bộ dụng cụ biên dịch — rồi chạy từ nguồn. Khi nào có bản dựng sẵn, bước này sẽ mất đi.

Vào [go.dev/dl](https://go.dev/dl/), tải bản cho máy bạn, cài như mọi phần mềm khác. Xong thì mở Terminal (hoặc PowerShell trên Windows) và gõ:

```
go version
```

Thấy một dòng có số phiên bản là được.

## 2. Chọn chỗ model chạy

Mặc định, W.O.N nói chuyện với một model **nằm ngay trong máy bạn**. Không sửa gì cả thì câu bạn gõ không đi đâu khỏi máy. Đó là chỗ chữ của bạn vốn ở, không phải một tuỳ chọn bạn phải bật. Muốn tự kiểm thì rút mạng ra: hệ vẫn chạy.

Nên bước này chỉ có hai việc nhỏ. Cài [Ollama](https://ollama.com) (chương trình lo việc chạy model trên máy) rồi tải một model về:

```
ollama pull qwen3.5:9b
```

`qwen3.5:9b` là gợi ý cho model chính: model này đứng đầu hạng dưới 9B ở khả năng tuân thủ chỉ thị, nói được tiếng Việt và máy tầm trung chạy được. Máy yếu hơn thì `gemma4:e4b` nhẹ hơn khoảng một nửa. Bạn quen model local nào khác thì cứ dùng model đó. Chỉ cần nhớ tên bạn vừa tải, mục 4 sẽ cần.

Ba kẻ đứng bờ (mục 6) dùng một model nhỏ riêng, mặc định `qwen3.5:4b`. Muốn dùng chung một model cho cả hai thì đổi `model` trong `won.conf`.

Vậy là xong, sang mục 3 được rồi.

### Nếu muốn một model thông minh hơn

Model nằm trong máy thì kín và không tốn tiền, nhưng chưa sắc bằng mấy model lớn ngoài kia. Muốn đổi thì có bốn chỗ, xếp từ kín nhất xuống. Càng xuống dưới model càng khá, và bạn càng phải tin thêm một người:

| Chỗ | Ai đang giữ chữ của bạn | Đổi thế nào |
|---|---|---|
| **Trong máy bạn** | không ai cả | mặc định, khỏi làm gì |
| **Model lớn, vẫn vào bằng cửa Ollama** | Ollama: họ viết rõ là chỉ giữ trong lúc trả lời, không dùng chữ của bạn để dạy máy | đăng nhập `ollama signin`, rồi chọn tên model có đuôi `:cloud`, ví dụ `kimi-k3:cloud` hoặc `glm-5.2:cloud` |
| **Một chỗ trung chuyển** (OpenRouter) | chỗ đó: mặc định họ không lưu gì và bật được luật *chỉ đi qua nơi nào không lưu* | bỏ dấu `#` ở dòng có `openrouter` |
| **Nhà cung cấp, đi thẳng** (Anthropic, OpenAI, Google…) | chính họ. Họ không lấy chữ của bạn dạy máy, nhưng muốn họ **không lưu** thì phải hỏi xin riêng | bỏ dấu `#` ở dòng của họ, hoặc thay bằng địa chỉ nhà bạn tin |

Hàng ba kín hơn hàng bốn ở một chỗ: chỗ trung chuyển không biết bạn là ai, còn đi thẳng thì nhà cung cấp có danh tính của bạn.

Chỗ để đổi nằm trong thư mục `.system/proxy/`: copy file `won.conf.example` ra một bản, **đổi tên thành `won.conf`** (bỏ chữ `.example` đi). Mở nó ra, ở mục **UPSTREAM** có năm dòng đã nằm sẵn, cả năm đang tắt:

```
# upstream = http://127.0.0.1:11434
# upstream = https://openrouter.ai/api
# upstream = https://api.anthropic.com
# upstream = https://api.openai.com
# upstream = https://generativelanguage.googleapis.com
```

Bỏ dấu `#` ở **đúng một dòng** rồi lưu. Ví dụ chọn chỗ trung chuyển thì dòng đó thành:

```
upstream = https://openrouter.ai/api
```

Còn để nguyên cả năm dòng đang tắt thì model chạy trong máy bạn.

Hai điều nên biết trước khi trao chữ mình cho ai:

- **Chính sách của mỗi nhà thay đổi theo thời gian.** Trang nói về quyền riêng tư của họ là chỗ để bạn tự kiểm và đáng đọc một lần trước khi chọn.
- **Đổi chỗ thì nhớ đổi cả tên model.** W.O.N chỉ lo chữ *đi tới đâu*; còn *model nào* thì agent AI của bạn tự gọi tên, và W.O.N không sửa cái tên đó. Mỗi agent có chỗ chọn model riêng. Mục 4 chỉ cách hỏi ra chỗ ấy.

## 3. Chạy

Mở Terminal ở thư mục W.O.N, rồi:

```
cd .system/proxy
go run .
```

Lần đầu hơi lâu. Khi thấy dòng `listening` là proxy đang chạy. Cứ để đó, đừng tắt cửa sổ.

Muốn dừng: `Ctrl + C`. Sửa `won.conf` xong thì dừng rồi chạy lại: proxy chỉ đọc file lúc khởi động.

## 4. Chỉ đường cho agent AI

Bạn vừa mở một cái cửa ngay trong máy mình, ở địa chỉ này:

```
http://127.0.0.1:8787
```

Việc còn lại là bảo agent AI bạn đang dùng gửi câu hỏi qua cái cửa đó, thay vì gửi thẳng ra ngoài. Mỗi agent khai một kiểu: có cái cho sửa thẳng trong cài đặt (chỗ đó thường tên là *base URL*, *endpoint*, hay *địa chỉ API*); có cái lại đọc từ một biến môi trường của máy.

**Cách nhanh nhất là nhờ chính agent đó chỉ chỗ.** Bạn hỏi nó nguyên câu này:

> Tôi đang chạy một proxy ở `http://127.0.0.1:8787`. Chỉ tôi chỗ đổi địa chỉ model (base URL / endpoint) trong agent này, từng bước. Và chỉ luôn chỗ chọn tên model.

Agent nào cũng biết cài đặt của chính nó rõ hơn tờ giấy này.

⚠️ Nếu agent của bạn đọc **biến môi trường**: trên Windows, biến vừa đặt chỉ có tác dụng với cửa sổ mở **sau** đó. Nên đặt xong thì mở Terminal mới, hoặc khởi động lại app.

Muốn thôi thì trả cái địa chỉ đó về như cũ (hoặc xoá biến vừa đặt). Mọi thứ về đúng như trước.

Xong. Từ giờ mọi câu bạn hỏi đều đi qua đây, và hệ đi cùng. Mặc định bạn đang nói với **Tzu**, đệ điều phối.

Muốn biết chắc là đã trúng thì có hai dấu. Trong Terminal đang chạy proxy, dòng khởi động phải có `default_agent=Tzu`, và mỗi câu bạn gõ phải làm hiện thêm một dòng. Rồi hỏi agent một câu bất kỳ: nếu nó biết bạn đang ở trong W.O.N mà bạn không hề nói, tức hệ đã đi cùng.

Muốn gọi đệ khác thì đổi dòng `default_agent` trong `won.conf` (`Sun`, `Mo`, `Fan`, `Han`, `Shu`). Agent nào cho bạn chọn "agent" ngay trong ô chat thì khỏi cần đổi gì, chọn ở đó là xong.

## 5. Việc thật bắt đầu ở đây

Trong thư mục có **mười một file trống**, ở `What/`, `Own/` và `Need/`. Chúng trống có chủ ý. Chữ trong đó phải là chữ của bạn, không ai viết hộ được và cũng không nên có chữ mồi sẵn để bạn sửa theo.

Viết tới đâu, hệ hiểu bạn tới đó. Cứ viết một dòng thôi cũng được: hôm nay một dòng, mai thêm một dòng.

Muốn biết viết gì vào đâu thì [README.md](README.md) nói rõ từng trụ.

---

## 6. Ba kẻ đứng bờ

Phần này **mặc định bật**, nhưng họ chỉ nói được khi máy bạn có model ở mục 2. Chưa tải model thì họ im, và hệ vẫn chạy đủ.

Đôi khi sau câu bạn hỏi sẽ có thêm một dòng ngắn mang một dấu:

> 🚶 *một góc nhìn khác về chuyện bạn đang làm*
> 🧰 *một món công cụ đang nằm sai chỗ*
> 🛣️ *một mốc thời gian: bạn đã đi bao xa*

Họ **chỉ nói, không làm gì cả**, và phần lớn lượt thì họ im.

Một điều đáng biết trước: **đệ bạn đang nói chuyện cũng nghe được mấy dòng đó.** Nên có lúc chính đệ ấy sẽ nhắc lại ("kẻ tạt ngang vừa nói một câu, tôi thấy đúng chỗ này") hoặc trả lời lại. Đó không phải lỗi. Ba kẻ đứng bờ nói *vào phòng*, không nói riêng với bạn, và bạn được nghe cùng lúc với đệ trong phòng.

### Cần gì để họ nói

Ba kẻ này nghĩ bằng một model riêng, **chạy trên máy bạn**, không phải model chính của phiên. Chỉ cần cài [Ollama](https://ollama.com) rồi tải model về.

```
ollama pull qwen3.5:4b
```

Trong `won.conf`, mục **LOCAL MODEL** đã khai sẵn đúng model đó, và mục **PLUGINS** đã bật sẵn cả ba:

```
loiterer.enable = true
outfitter.enable = true
wayfarer.enable = true
```

Muốn kẻ nào nghỉ thì đổi dòng của kẻ đó thành `false`. Các dòng tuỳ chọn của họ cứ để nguyên, hệ bỏ qua khi đã tắt.

Chậm hơn model lớn một chút, nhưng **không tốn token, và không câu nào ra khỏi máy bạn**. Ba kẻ đứng bờ chỉ trả một dòng mỗi lượt, nên model nhỏ là vừa đủ.

Khai một model `:cloud` ở đây thì được, nhưng ba kẻ này **đọc một bản sao hội thoại của bạn** để có gì mà nói. Nên chỉ cần một cái tên `:cloud` ở dòng `model` là bản sao ấy cũng đi ra ngoài, kể cả khi model chính vẫn đang chạy trên máy.

Dừng proxy rồi chạy `go run .` lại; dòng khởi động có tên cả năm plugin là xong.

---

## Nếu có gì không ổn

**Chưa chạy được.**

| Bạn thấy | Làm gì |
|---|---|
| `go: command not found` | Go chưa cài xong, hoặc cần mở lại Terminal |
| Dòng `plugins skipped ... built=0` | Gõ `go generate ./...` rồi chạy lại |
| Báo lỗi `502` | Không ai trả lời ở chỗ bạn chọn. Dòng lỗi nói rõ chỗ nào. Nếu đang chạy trên máy thì Ollama chưa mở: bật nó lên |
| Chỉ hỏng khi model chạy trên máy | Nâng Ollama lên bản mới nhất |

**Chạy rồi mà sai chỗ.**

| Bạn thấy | Làm gì |
|---|---|
| `model ... not found` | Tên model agent AI đang gọi không có trên máy. Gõ `ollama list` xem đang có gì, rồi khai lại tên (mục 4) |
| Báo lỗi `400 ...` về host | Agent của bạn đang tự trỏ đi một chỗ khác chỗ bạn khai. Đổi dòng trong `won.conf` cho khớp, hoặc để agent trỏ về cửa của W.O.N |
| Sửa `won.conf` mà không thấy gì đổi | Phải dừng (`Ctrl + C`) rồi chạy lại; file chỉ đọc lúc khởi động |
| Trỏ đường rồi mà **không thấy gì khác** | Xem Terminal: `default_agent=Tzu` phải có ở dòng khởi động. Nếu là `(none…)` thì `won.conf` đang đặt `default_agent = off` |
| Đệ trả lời **trớt quớt**, như chưa đọc gì về bạn | Chỗ đọc của model đang hẹp, nên phần đầu bị bỏ mà không báo gì. Chạy lại Ollama với chỗ đọc rộng hơn: `OLLAMA_CONTEXT_LENGTH=64000 ollama serve`. Máy yếu thì đổi sang model lớn ở mục 2: chỗ đọc của nó vốn đã rộng |

**Chậm, hoặc im.**

| Bạn thấy | Làm gì |
|---|---|
| Lượt nào cũng chậm thêm vài giây | Mở `won.conf`, siết riêng kẻ hay chậm bằng `<tên>.budget_ms` (ví dụ `wayfarer.budget_ms = 3000`), hoặc hạ `timeout_ms` |
| Agent **treo**, không lỗi gì | Xem Terminal: dòng `upstream error status=429` là nhà cung cấp đang chặn nhịp, agent tự chờ rồi thử lại |
| Ba kẻ đứng bờ luôn im | Bình thường. Hoặc chưa khai `model` ở mục 6 |

Không đâu vào đâu thì cứ tắt đi. W.O.N trên giấy không cần cái này.

---

## Các dòng trong `won.conf`

**Xoá cả file đi thì hệ vẫn chạy, ngay trên máy bạn.** Mọi dòng đều là tuỳ chọn.

Khi mở file ra:

- mọi dòng có nghĩa đều là `tên = giá trị`
- plugin bật tắt bằng `<tên plugin>.enable = true` hoặc `= false`
- dòng mở đầu bằng `# |` là **lời chú giải**: đọc rồi bỏ qua, máy không đọc
- dòng mở đầu bằng `#` rồi tới tên một núm (một dòng cấu hình) là **một cấu hình đang tắt**; bỏ dấu `#` đi là bật
- mọi dòng đang bật trong `won.conf.example` đúng bằng mặc định của hệ

### Phần chính

| Dòng | Mặc định | Nghĩa |
|---|---|---|
| `upstream` | `http://127.0.0.1:11434` | chữ của bạn đi tới đâu; để trống là chạy trên máy bạn |
| `listen` | `127.0.0.1:8787` | địa chỉ cái cửa vừa mở |
| `control` | `127.0.0.1:7777` | buồng lái local để xem trạng thái; `off` là tắt hẳn |
| `ground` | README + ba trụ | file nào được đọc để hệ biết bạn là ai; `off` = không đọc gì |
| `default_agent` | `Tzu` | đệ dùng khi agent không nói đang gọi ai; `off` = không chèn gì |
| `strip_tags` | ba tag đã biết | khối của agent AI cần **bỏ trọn** (ví dụ `system-reminder`) |
| `unwrap_tags` | `userRequest` | khối cần **giữ ruột, bỏ vỏ**; câu của bạn nằm bên trong |
| `strip_sections` | `Tone and style, Text output, auto memory` | mục trong lời dặn của agent AI cần cắt, theo tiêu đề |
| `strip_identity` | `You are ` | gỡ *câu* khẳng định vai mở đầu bằng chuỗi này; khớp ở đầu khối hoặc đầu dòng |
| `total_budget_ms` | `60000` | chờ tối đa cho **cả** một lượt; mọi plugin cộng lại |
| `log_level` | `info` | **chữ ra màn hình**: `info` mỗi request một dòng · `silent` im hẳn · `debug` kể từng bước |
| `debug_log` | chưa khai | **file lưu xuống máy**: `true` thì mỗi request được ghi vào `run/` để lúc trục trặc còn cái xem lại. File đó chứa đúng những gì bạn gõ. Chưa khai = chỉ ghi khi `log_level = debug` |

### Model cho bốn plugin

| Dòng | Mặc định | Nghĩa |
|---|---|---|
| `model` | `qwen3.5:4b` | tên model: model local nào bạn quen cũng được; để trống thì bốn plugin không gọi model nào |
| `base_url` | `http://127.0.0.1:11434` | địa chỉ Ollama trên máy bạn |
| `timeout_ms` | `15000` | chờ tối đa cho **một** lượt hỏi model |
| `max_tokens` | `160` | trần số chữ model được sinh ra trong một lượt; họ trả một dòng nên không cần nhiều |
| `think` | `false` | tắt phần model tự nghĩ trước khi nói; bật thì cần chờ trả lời lâu hơn, và có model không hỗ trợ |
| `temperature` | `0.4` | độ sáng tạo: thấp thì an toàn và hay lặp, cao thì đa dạng hơn nhưng dễ lệch khuôn; `off` = bỏ núm này |
| `keep_alive` | `30m` | giữ model nằm sẵn trong bộ nhớ bao lâu sau một lượt hỏi |

### Ai đang bật

Mỗi dòng là một plugin, và cả năm đều bật sẵn. Đổi `true` thành `false` là plugin đó nghỉ. Trong file, mục **PLUGINS** có năm dòng này, mỗi dòng kèm một câu giải thích phía trên:

```
identity.enable = true
memory.enable = true
loiterer.enable = true
outfitter.enable = true
wayfarer.enable = true
```

Vài lựa chọn nhỏ cho từng plugin:

| Dòng | Mặc định | Nghĩa |
|---|---|---|
| `loiterer.faces` | `anh xe ôm, cô bán nước, chú bảo vệ` | khuôn mặt để kẻ tạt ngang mang, xoay vòng theo từng lần chạy; `off` = ghé vô danh |
| `outfitter.min_turns` | `3` | bạn chưa nói đủ mấy lượt thì chưa lên tiếng |
| `memory.stone_weight` | `10` | mỗi lần xác nhận cộng bao nhiêu điểm cho một trang ký ức |
| `memory.max_index_per_zone` | `20` | trần số dòng index cho mỗi vùng ký ức |
| `memory.scorer` | `Shu` | đệ được phép ghi điểm cho trang ký ức |
| `memory.max_open_per_turn` | `2` | mở nhiều nhất mấy trang, trong lần mở duy nhất của mỗi buổi |
| `<tên>.budget_ms` | *theo trần chung* | dùng được với **mọi** plugin: chờ tối đa bao lâu cho riêng plugin đó, tính từ lúc plugin ấy bắt đầu chạy; không nới quá trần chung |

Viết sai tên một dòng thì hệ báo ngay lúc chạy, nên cứ thử.

---

Vậy là xong phần máy. Phần đáng giá nằm ở mục 5, và nó không cần một dòng nào trong file này. Mở một trong mười một file trống ra, viết một dòng.
