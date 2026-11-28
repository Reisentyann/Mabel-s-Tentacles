---
cod-title: 计算领域联合检索与一致性报告
cod-domain: computing
cod-report-id: COMPUTING-REPORT-2026-11
cod-marker: SEARCH_LONG_COMPUTING_REPORT_2026
cod-locale: zh-CN
cod-scope: inverted index / cache invalidation / network protocol / sorting algorithm
---

# 计算领域联合检索与一致性报告

这份报告把倒排索引、缓存失效、网络协议和排序算法放在同一条检索链路上观察。重点不是把四个主题分别写成百科，而是说明一篇文档从写入、建立索引、进入缓存、跨网络传输到按稳定顺序返回时，哪些字段必须保持一致，哪些边界需要被明确验证。文中所有样例都使用中文叙述和重复出现的英文关键词，便于检索字段抽取模板同时识别自然语言正文与结构化键。

## 1. 范围、术语与证据模型

本次记录的检索域为 computing，时间基准为 2026-11-24，观察窗口覆盖一次完整写入和三轮查询。每份材料至少包含以下证据层级：

- cod-document-id 是文档身份，不因标题改写而改变。
  - inverted index 使用文档身份连接 posting list 与 term dictionary。
  - cache invalidation 使用文档身份定位需要删除或更新的缓存键。
  - network protocol 在请求与响应中回传文档身份，避免分片重组时错配。
  - sorting algorithm 在并列分数下用文档身份作为最终 tie-breaker。
- cod-topic 标记主主题，但一篇文档可以携带跨主题证据。
  - 主主题为 inverted index 时，正文仍可讨论 cache miss 对查询延迟的影响。
  - 主主题为 cache invalidation 时，正文仍可记录 protocol timeout 后的回源策略。
  - 主主题为 network protocol 时，正文仍可给出 stable sort 的验收条件。
  - 主主题为 sorting algorithm 时，正文仍可比较 index segment 合并顺序。
- cod-evidence 保存可复核的动作、输入、结果与反例，不使用“运行正常”这类无法定位的结论。
- cod-owner 指明字段责任人，cod-version 区分结构版本与业务版本，cod-review-state 表示是否进入联合回归。

> 检索质量并不只由 recall 决定。若 inverted index 能找到文档，而 cache invalidation 仍返回旧 payload，或 network protocol 丢失序号，最终结果依然错误。排序算法只能放大已经存在的偏差，不能修复上游数据。

## 2. 倒排索引：词项、位置与可见性

inverted index 的核心是把 term 映射到 document id 集合。第一组材料使用中文分词后的“缓存失效”，同时保留英文关键词 cache invalidation，用来验证同义词扩展不会把 protocol timeout 误当成同一主题。词项字典按字典序存放，posting list 除了文档编号，还携带 term frequency、field length 与可选 position。查询“网络协议 排序”时，系统先取两个词项的并集，再按 field boost 计算分数，不能简单地拼接两个列表。

| 字段 | 示例值 | 校验点 |
| --- | --- | --- |
| cod-index-term | cache invalidation | 同一个词项的大小写归一规则一致 |
| cod-posting-ref | doc-017:3,9,21 | 位置严格递增且不越界 |
| cod-document-frequency | 418 | 与当前可见 segment 的统计一致 |
| cod-score | 7.625 | 浮点结果按固定精度比较 |
| cod-visible-version | v36 | 已删除文档不得进入最终结果 |

段合并会把多个 immutable segment 重写为更大的 segment。合并期间，新的写入进入独立 segment，查询必须看到一致的 commit point。若删除标记尚未应用，旧 posting 仍可能短暂出现；因此 cod-visible-version 与 cod-delete-marker 必须一起检查。高亮字段应基于实际 position 截取，不能因为 payload 被裁剪就改变短语判断。对于“缓存 失效”和“缓存失效”是否等价，应由 analyzer 版本明确说明，而不是由排序结果反推。

嵌套列表还要求记录失败路径：

- 查询入口
  - 词项归一化
    - 中文分词边界
    - 英文 keyword 小写化
  - 布尔求值
    - AND 使用交集
    - OR 使用去重并集
  - 打分与排序
    - BM25 归一化
    - stable sort 保持同分顺序
- 可见性入口
  - commit point 之前不可见
  - delete marker 优先于 posting
  - segment merge 不重复计数

## 3. 缓存失效：版本、广播与回源

cache invalidation 的目标是让读取方在可接受的时间窗口内观察到新版本，同时避免所有请求同时回源。材料采用“先提交数据库，再失效缓存”的顺序，并用 cod-cache-generation 表示缓存代际。写入成功后发布包含 document id、generation 与 topic 的失效消息；消费者只有在本地 generation 落后时才删除键。若消息重复投递，删除操作必须幂等。若消息丢失，定期对账会按版本差异补发，不能依赖单次广播保证最终一致。

热点文档在失效瞬间可能产生 cache stampede。请求合并器把同一键的并发读取折叠为一次回源，其余请求等待同一个结果。等待必须有 deadline，否则网络抖动会把缓存问题转化为线程池耗尽。负缓存适合短期抑制不存在的文档，但 TTL 过长会让新建文档长时间不可见。材料建议对不存在结果使用较短窗口，并在写入成功后主动清除对应的 negative cache entry。

回源路径还要处理延迟双删的竞态：第一次删除后，旧回源请求可能刚把旧值写回；第二次删除用于清理这种回填，但第二次删除同样可能早于慢请求。更稳妥的做法是回源前读取 generation，写入缓存时比较 generation，只允许当前代际覆盖。cod-cache-policy 因而不仅记录 TTL，还记录版本条件与回源开关。标签失效适合批量更新 analyzer 或 field boost，但依赖图必须防止循环失效。

## 4. 网络协议：分帧、重试与背压

network protocol 段落关注应用层消息边界。长度前缀可以解决 TCP 粘包，却仍要限制 maximum frame size，避免异常长度触发超大分配。消息头包含 protocol version、request id、sequence、payload length 与 checksum；接收端先校验长度和校验和，再反序列化业务字段。cod-sequence 在单个连接内递增，跨连接时使用 stream id 区分，不能把两路流的序号混为一个计数器。

超时预算需要分层。connection timeout 只覆盖建连，read timeout 覆盖等待响应，总 deadline 覆盖重试链。若每次重试都重新开启完整预算，用户等待可能成倍增加。材料约定最多两次 retry，并使用 exponential backoff with jitter；只有可重试错误才进入重试，参数错误与权限错误立即返回。写操作携带 cod-idempotency-key，服务端在去重窗口内保存请求摘要与响应摘要，重复请求返回首次结果。

流控与背压是另一组边界。发送方维护滑动窗口，接收方通过 window update 通知可继续发送的字节数。若应用消费速度低于网络速度，buffer 会增长，应按高水位暂停读取、低水位恢复，而不是无限制排队。分片重组需要按 sequence 排序并检测空洞；超过重组 deadline 的分片应被丢弃，同时释放 buffer。证书轮换和连接复用不得改变 request id 的语义，监控指标也要区分 connect、send、wait 与 consume 四个阶段。

## 5. 排序算法：全序、稳定性与资源上限

sorting algorithm 的选择取决于数据规模、内存预算和是否需要稳定性。比较器必须构成全序：先按 score 降序，再按 document id 升序，最后按 version 降序。若比较器对两个不同对象返回零，stable sort 可以保留输入顺序，但输入顺序若来自并发 segment 合并，结果仍可能波动。因此 cod-sort-key 应明确包含所有决胜字段，而不是只写“按相关度排序”。

稳定归并适合链表或外部排序，额外空间与链表节点数成正比。快速排序通常原地且缓存友好，但三路划分能减少重复 key 带来的交换；重复 score 很多时，应比较普通两路划分与 Dutch national flag 的效果。堆排序适合 TopK 和内存受限场景，时间复杂度稳定，但常数开销可能高于归并。基数排序适用于可无损编码的整数或定长字符串 key，必须验证编码保持原有顺序，不能直接把中文 UTF-8 字节当作业务排序键。

外部排序先产生多个有序 run，再按归并策略合并。run 数量过多时采用多路归并或分层合并，避免一次打开全部文件。排序后的校验包括 key 非降序、重复对象去重、总数守恒和游标可继续。游标应保存最后一条完整 sort key，而不是保存易变的 offset；并发写入时，offset 分页可能漏项或重项。cod-page-cursor 因而比 cod-page-offset 更适合联合检索结果。

## 6. 联合回归、结构化样例与交付约束

联合回归按四个主题交叉构造样例。倒排索引样例负责找全文档，缓存失效样例负责让文档内容保持最新，网络协议样例负责按序送达，排序算法样例负责稳定输出。每一轮都记录输入规模、命中数量、缓存命中率、重试次数、排序比较次数和端到端延迟，避免用单一指标掩盖问题。

```json
{
  "cod-report-id": "COMPUTING-REPORT-2026-11",
  "cod-topics": ["inverted index", "cache invalidation", "network protocol", "sorting algorithm"],
  "cod-assertions": {
    "cod-index-visible": true,
    "cod-cache-generation-current": true,
    "cod-protocol-sequence-contiguous": true,
    "cod-sort-total-order": true
  },
  "cod-result": "四个主题的字段能够被同一检索模板抽取并交叉验证"
}
```

上线前应完成以下检查：

- [x] inverted index 的 posting 数量与可见文档统计一致。
- [x] cache invalidation 的旧代际不会覆盖新代际。
- [x] network protocol 的重复帧被去重，丢片被拒绝。
- [x] sorting algorithm 在重复分数下仍产生确定顺序。
- [ ] 将 [缓存失效检查表](cache-invalidation-checklist.md) 纳入下次演练。

最终交付物保留旧小样本不变，只新增 computing 领域长文本及其 manifest。manifest 中的 path、bytes、sha256 与 marker 用于精确定位文件；expected_fields 使用完整 cod-* 字段名；expect 只保存可由模板直接计算或精确比较的值；expected_absent_fields 用于防止凭据、环境变量和无关运行日志混入检索语料。
