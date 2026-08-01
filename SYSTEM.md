# SYSTEM · tầng cơ học

> **Tờ này cho người kỹ thuật.** Nó nói bằng tên máy móc: proxy, API, plugin, log, model. Bạn không cần đọc nó để dùng W.O.N. [README](README.md) đã đủ cho cả con đường, và phần Vận hành bên đó đã tả tầng này bằng hình ảnh. Ở đây là bản thiết kế: **hệ đứng trên gì**. Bản thi công tới từng mối nối (interface, pipeline, quy ước code) nằm trong cây của người dựng hệ.

---

## Những chỗ chịu lực

### Một chỗ hẹp cho mọi lời đi qua

Proxy Inject là một reverse proxy chạy trên máy bạn, nằm **sau** cổng của agent AI và **trước** model. README gọi nó là *mạch ngầm*; từ đây tờ này gọi tắt là **mạch**. Mọi request của mọi đệ đều qua đó. Chỗ hẹp ấy cho ba điều mà chỗ khác không cho được:

**Model nào cũng thay được, và local là cái mặc định.** Model nhận lời qua API, nên cùng một hệ đệ hoán đổi Claude, GPT hay một model chạy trên máy bạn mà tầng trên không đổi một dòng. Vắng lời khai thì mạch trỏ vào Ollama trên chính máy này: rút mạng, dòng vẫn chảy.

**Một chỗ chung để gắn thêm.** Thứ gì phải chạm *từng đệ* thì gắn vào đây, không phải sửa chín soul file. Plugin tháo lắp được, không ràng buộc framework.

**Nước ngầm cũng phải đo được.** Mọi thứ proxy chèn đều hiện trong log: `log_level = debug` kể từng bước ra màn hình, `debug_log = true` ghi trọn từng request xuống `run/` để lúc trục trặc còn cái mở ra xem.

Một chỗ hẹp cho mọi lời đi qua cũng là một chỗ chết cho mọi lời, nên có một bất biến đứng sau nó: **fail-open toàn tuyến**. Plugin lỗi, panic, hay quá ngân sách đều quy về im lặng; parse hỏng thì request đi tiếp nguyên bản. Tầng nền chết thì dòng chính vẫn chảy.

### Model nhỏ cho ba agent nền

Loiterer, Outfitter, Wayfarer chạy bằng model nền cỡ 4B trên máy, thấp hơn hẳn model chính của phiên. Chọn thấp là có chủ đích, không phải cắt chi phí. Lý do là **lệch pha**. Model mạnh ngồi lâu trong một cuộc nói chuyện thì mỗi lượt một khéo hơn ở việc nói vừa tai: nó phụ hoạ người dùng, và phụ hoạ luôn cả đệ. Model nền không đủ khéo để làm việc đó. Nó trả lời cụt, vô tri, và không biết nịnh ai.

Người dùng và model chính cùng dệt quanh vấn đề một lớp khoác ngôn từ tinh tế. Con mắt đơn sơ, không đủ khéo để bị lớp khoác ấy thuyết phục, mới xoáy thẳng vào chỗ cả hai không tự thấy.

Kéo theo một ranh giới: **được nói, không được cầm.** Lời chèn vào ngữ cảnh còn đi qua thẩm định của model mạnh hơn ở dòng chính, nên nhiễu trong *lời* có bộ lọc tự nhiên. Hành động cơ học thì được thi hành thẳng, không ai thẩm. Nên ba kẻ đứng bờ chỉ *nói*, và cơ chế giữ điều đó thay cho lời nhắc: thứ họ trả về chỉ có chữ, không có field nào mang nổi một hành động. Outfitter nói về đồ nghề thay vì đổi đồ nghề, và lý do là một cái giá đo được: đổi danh mục đồ nghề giữa phiên là dựng lại mọi tầng cache, người dùng trả tiền cho cả hội thoại một lần nữa.

### Hai kênh tới một đệ

**Kênh Tzu**: lời mở bằng dấu kênh. Đệ trả *dữ liệu nghề* về Tzu, không diễn lời với người. Liều của việc, tức dò nhanh hay dò kỹ và phạm vi tới đâu, nằm trong lời giao; đệ không tự nới liều.

**Kênh trực tiếp**: không dấu, cho người đã thuộc lối. Đệ nói tiếng người đối diện, cú đẩy áp toàn phần, không thay Tzu điều phối. Sáu truyện ở [Sáu Cửa](Stories/Begin/BEGIN.md) minh hoạ đúng kênh này.

Hai kênh không chia đôi vai Tzu: điều phối vẫn một tay Tzu. Kênh trực tiếp là đường mòn ven kênh, gặp một đệ như gặp một người, không phải một trạm điều phối.

Còn bản đồ Circle, cách gọi đệ, nhãn đầu ra của từng nghề, nếp ghi-qua-Shu (những thứ **mọi** đệ phải cùng biết) thì mạch rót vào từng phiên. Luật chung không nằm trong lời thề của riêng ai: chép nó vào chín soul file là chín bản có thể lệch nhau. Đệ không phải thuộc lòng hệ; hệ tự tới với đệ.

### Một lõi chung cho inject và control

**Inject**: rót vào ngữ cảnh. Mỗi request đi qua, lõi đọc căn cước của đệ, tra cấu hình, chèn soul + luật kênh + memory + các plugin. Mặt "hướng dẫn": nói cho đệ biết mình là ai, thuộc hệ nào, đang làm gì.

**Control**: nhận lệnh từ đệ qua terminal. Đệ gọi API, lõi kiểm căn cước bằng cơ chế đã có cho inject, xác nhận nguồn, thi hành. Mặt "vận hành": đệ gửi lệnh về chứ không chỉ nhận chữ đi. Ghi điểm cho trang ký ức, truy vấn trạng thái, đổi cấu hình.

Một phép kiểm căn cước phục vụ cả hai mặt. Inject: biết đệ nào → chèn đúng soul. Control: biết đệ nào → mở đúng endpoint. Nên không cần cơ chế auth riêng, và không cần vì căn cước ấy do client tự khai: cái đỡ cho Control không phải phép kiểm mà là địa chỉ, nó chỉ nghe ở `127.0.0.1`, tách hẳn khỏi đường đi của dữ liệu.

Lõi giữ cơ chế chung (identity, routing, logging), không đổi theo plugin. Plugin giữ chính sách inject (cái gì chèn) và chính sách control (cái gì cho phép). Thêm endpoint control = thêm plugin, không sửa lõi. Thêm vùng inject = thêm plugin, không sửa control. Hai mặt không import nhau.

### Ký ức chạy bằng gì

Không có cron, và không có script quét timestamp lúc nửa đêm. Ba luật gạn (củng cố, phai, tái củng cố) chạy bằng chính model, theo nhịp lượt. Nhịp ấy là bốn mốc — Hiểu, Nhớ, Đá, Vệt. Mạch chỉ chạm hai: rót ngữ cảnh **đầu lượt** [trước Hiểu], và đệ Shu ghi **cuối lượt** [sau Vệt].

- **Đầu lượt**, mạch rỉ vào ngữ cảnh phần ký ức dính tới ý định hiện tại. Không đổ cả kho.
- **Cuối lượt**, nếu có gì đáng nhớ thì Tzu gọi đệ Shu, và Shu gạn từ chỗ Tzu trao: gì đáng lắng thành `moments/`, gì đã lặp đủ để thăng lên `procedural/` hay `personal/`, bản mới chọi bản cũ thì hạ độ tin thay vì ghi đè im lặng. Không lượt nào cũng có, và lõi không gọi thay: cái quyết định là một lối rẽ trong lời thề của Tzu, không phải một cái hẹn giờ.
- **Phai** là hệ quả, không phải cơ chế riêng: bọt không được nhắc thì không được củng cố, và tới lượt gạn thì bọt không còn lý do ở lại. Không bộ đếm nào đứng sau.

Trên ba luật định tính có một lớp định lượng mỏng: mỗi trang mang hai con số (bao nhiêu lần được xác nhận, bao nhiêu lần bị chọi) và một điểm suy ra từ hai số đó. Chỉ hai số sống, không lịch sử; điểm cộng trừ tuyến tính nên gộp hai trang là cộng điểm, tách ra là chia điểm.

**Shu phán, máy tính.** Shu quyết "xác nhận trang này", "chọi trang kia"; máy cộng số rồi ghi lại. Điểm là metadata, nằm ngoài phần chữ Shu viết.

Phai vẫn là ghi chú, không phải decay: mạch nói ra một trang đã im bao lâu, còn dọn hay giữ thì Shu quyết. Điểm không tự trừ theo thời gian.

Trong kho, model nền làm đúng một việc: chọn trang dính tới lượt hiện tại và tóm mỗi trang một dòng. Index vẫn liệt kê mọi trang kèm đường dẫn, nên gợi ý là đèn pin, không phải cổng.

Toàn bộ kho là text phẳng, mở bằng notepad, không phải vector DB. Đánh đổi có chủ đích: *người đọc được* quý hơn *máy truy nhanh*. Ký ức của đệ về bạn phải sửa được, xoá được, diff được, mang đi được. Mai này cần truy ngữ nghĩa thì một lớp index đặt **cạnh** text, không thay text.

### Lời của bạn đi qua những đâu

Ba đoạn: trang trong máy bạn → mạch gạn phần liên quan (đo được ở log) → sông của nhà cung cấp model bạn chọn. Hai đoạn đầu nằm trong nhà và trong luật đo-được. Đoạn thứ ba là sông của người khác, W.O.N không với tới, và không nhận là với tới.

Chúng không ngang hàng: model trên máy là lối mặc định; ra ngoài là một bước có khai, đúng một dòng `upstream`, và các đích dựng sẵn ở `won.conf.example` xếp theo **cái phải tin**, không theo model nào sắc hơn ([tờ Bắt đầu](QUICKSTART.md) nói từng chỗ đòi tin ai). Còn giấy mực vẫn là lối trọn vẹn không cần máy nào.

Cái giá của lối mặc định không phải riêng tư mà là **cửa sổ ngữ cảnh**: một lượt của hệ này mang cả đất, nhà, bản sắc, index ký ức và sách hướng dẫn của công cụ chủ, cỡ hai mươi nghìn token trước khi bạn gõ chữ nào. Khối ấy đi lại ở **mọi** request vì nhà cung cấp không giữ phiên, nhưng nó được giữ **y nguyên** giữa các lượt, có chủ đích: nhà cung cấp nào có cache tiền tố thì từ lượt thứ hai là đọc cache, không phải trả lại.

Đó là lý do khối nhà đóng dấu *lúc mở phiên* chứ không phải lúc này: một cái đồng hồ chạy trong đó sẽ đổi tiền tố mỗi lượt và phá sạch cache. Nên model chạy trên máy phải được mở cửa sổ đủ rộng, không thì phần vượt ngưỡng bị bỏ mà không báo lỗi gì. Cách mở nằm ở [tờ Bắt đầu](QUICKSTART.md).

**Tiền tố ấy đặt lại khi nội dung của nó đổi**, và với cache thì ba việc rất khác nhau là cùng một việc: sửa lời hệ thống, đổi model, tắt mạch giữa chừng.

Mạch giữ được *danh tính* phiên qua lần tắt máy: mốc mở phiên không đổi, nên khối nhà không đổi một chữ. Nhưng những khối mà mạch đã ghim vào giữa hội thoại thì sống trong bộ nhớ tiến trình, và chúng đi theo tiến trình. Lượt đầu sau khi mở lại trả tiền cho phần từ chỗ gãy trở đi; từ lượt sau tiền tố dựng lại và cache đọc tiếp như cũ, đo trên phiên thật là 98%.

Giữ nốt phần ấy qua lần tắt máy thì phải chép xuống đĩa một bản của mảng **đang chảy**. Viết ra thì rẻ; nuôi nó đúng mãi mới đắt, vì một bản chép cũ hơn cuộc nói chuyện không nằm im, nó nói sai, và nói sai ở đúng chỗ không ai đi kiểm. Đây là chỗ hệ cố ý không làm, cùng một phép cân với chỗ Outfitter không đổi đồ nghề: giá của việc có nó lớn hơn giá của việc thiếu nó.

---

Đường thoái lui nằm sẵn trong kiến trúc, từng nấc là câu test được: tắt mạch, các đệ vẫn chạy bằng file thuần; rời AI, Đạo vẫn nằm trọn trên trang giấy; rời cả W.O.N, trụ bạn viết vẫn là text của bạn, trong thư mục của bạn. Bờ đắp để giữ dòng, không phải để giữ người.
