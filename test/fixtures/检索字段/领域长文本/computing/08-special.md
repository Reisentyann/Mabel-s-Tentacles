# 计算检索字段特殊结构样本

cod-domain: computing
cod-marker: SEARCH_LONG_COMPUTING_SPECIAL_2026
cod-layout: 三十一个独立二级章节，每章包含唯一链接、空代码围栏与一张表格

本文件用于验证 Markdown 结构计数、唯一性约束与跨主题字段抽取。倒排索引章节关注词项、位置和段可见性；缓存失效章节关注代际、回源和容量；网络协议章节关注分帧、重试和背压；排序算法章节关注全序、稳定性和外部归并。每章的空代码围栏不承载代码，表格只保存可直接比对的字段值，链接全部使用不同的保留示例域名路径。

## 01 词项词典与文档频率

inverted index 的 term dictionary 需要同时保存中文分词结果与英文 keyword。document frequency 只统计当前可见 segment，删除标记应用后再计数。缓存失效可能改变 payload，却不应无意义地重建全部 posting；只有 analyzer 或字段内容变化时才触发受影响词项更新。
https://computing.example.test/catalog/01-term-dictionary
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-index-term | cache invalidation | 中英文词项映射一致 |
| cod-document-frequency | 418 | 与可见文档统计一致 |

## 02 倒排链表合并

多个 posting list 求交集时，较短链表优先推进，较长链表使用跳表跨过不可能命中的区间。短语查询还要比较 position 差值，不能只凭 document id 相交。网络协议按 sequence 重组分片与这一过程相似，但索引合并必须保持每个词项内的位置有序。
https://computing.example.test/catalog/02-posting-list-merge
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-posting-ref | doc-017:3,9,21 | 位置严格递增 |
| cod-query-mode | phrase intersection | 不把包含当作相邻 |

## 03 位置索引与短语查询

位置索引记录词项在字段中的偏移，短语“缓存失效”要求两个位置相邻。高亮根据实际 position 截取原文，payload 裁剪后仍要保留判断所需窗口。排序算法只能在候选集合上排序，不能弥补位置索引缺失造成的漏召回。
https://computing.example.test/catalog/03-positional-phrase-query
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-position-window | 8 | 覆盖目标短语 |
| cod-highlight-mode | positional | 不使用模糊字符串搜索 |

## 04 跳表加速并集

跳表在稀疏倒排链表上建立多层索引，求并集时跳过大量不可能产生新文档的节点。层高应随机且有上限，避免退化为线性扫描。缓存失效清除查询结果时，只需让当前 generation 失效，不必删除底层 immutable segment。
https://computing.example.test/catalog/04-skiplist-union
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-skiplist-level | 4 | 不超过配置上限 |
| cod-union-mode | deduplicated | 并集不重复文档 |

## 05 段合并与提交点

segment merge 把多个只读段重写为更大的段，新写入进入独立段。查询在固定 commit point 上读取一致快照，合并失败时保留旧段。网络协议的重组超时也采用类似原则：未完成的中间状态不得暴露给应用层。
https://computing.example.test/catalog/05-segment-merge-commit-point
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-commit-point | v38 | 单调推进 |
| cod-merge-state | immutable | 旧段在完成前保留 |

## 06 删除标记与可见性

删除标记与 posting 分开存储，查询在打分前过滤不可见文档。cod-visible-version 防止旧内容在刷新窗口内重新出现。缓存失效与索引删除必须共享 document id 和版本，否则可能找到文档却读取到已删除的 payload。
https://computing.example.test/catalog/06-delete-marker-visibility
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-delete-marker | applied | 打分前过滤 |
| cod-visible-version | v39 | 与缓存代际关联 |

## 07 权重归一化与 BM25

BM25 用 term frequency 与 field length 归一化，字段权重必须来自同一 analyzer 配置。document frequency 的变化会影响 IDF，因此段刷新后分数可能改变。排序算法使用完整 sort key 决胜，不能假设浮点分数永远唯一。
https://computing.example.test/catalog/07-bm25-normalization
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-score | 7.625 | 固定精度比较 |
| cod-field-boost | 1.4 | 配置版本一致 |

## 08 高亮与载荷裁剪

高亮需要真实 position，载荷裁剪不能删除短语判断所需的上下文。若响应只返回摘要，应在索引阶段保留独立 highlight field。网络协议分片不得截断 UTF-8 多字节字符，否则中文高亮会出现乱码。
https://computing.example.test/catalog/08-highlight-payload-trimming
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-highlight-field | body_zh | 使用独立字段 |
| cod-utf8-boundary | preserved | 不切断多字节字符 |

## 09 版本键与代际隔离

cache invalidation 使用 cod-cache-generation 区分新旧值，键中可包含业务版本，但代际必须单调。消费者处理乱序消息时，只接受不低于本地版本的事件。排序算法对事件按 generation 排序也不能替代接收端版本检查。
https://computing.example.test/catalog/09-generation-isolation
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-cache-generation | 132 | 单调递增 |
| cod-message-order | arbitrary | 接收端拒绝旧代际 |

## 10 写穿失效广播

写入成功后发布包含 document id、generation 与 topic 的失效消息，消费者删除对应键。重复广播必须幂等，失败消费者依靠定期对账补齐。若数据库提交成功但消息发送失败，不能直接回滚已经可见的索引段。
https://computing.example.test/catalog/10-write-through-invalidation
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-write-order | database then cache | 提交后失效 |
| cod-retry-mode | idempotent | 重复消息不扩大影响 |

## 11 延迟双删与竞态

延迟双删用于清理旧回源填回的缓存，但第二次删除仍可能早于慢请求。更可靠的方案是回源前记录 generation，写入缓存时比较版本。网络协议的超时取消不一定停止服务端处理，因此慢响应同样可能晚到。
https://computing.example.test/catalog/11-delayed-delete-race
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-source-generation | 133 | 回源前记录 |
| cod-write-condition | generation equal | 旧值禁止覆盖 |

## 12 热点回源与请求合并

singleflight 将同一键的并发 cache miss 折叠为一次回源，其余请求共享结果。等待必须受 deadline 约束，超时后允许后续请求重新竞争。排序算法不应让合并后的请求按随机到达顺序输出，document id 可作为稳定关联键。
https://computing.example.test/catalog/12-hotkey-singleflight
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-request-merge | singleflight | 同键一次回源 |
| cod-wait-deadline | 800ms | 超时不无限等待 |

## 13 负缓存与抖动窗口

negative cache 抑制不存在文档的重复回源，但 TTL 过长会让新建文档不可见。窗口应结合写入事件提前清除，并统计真实缺失与缓存缺失。网络协议的 retry jitter 也用于打散集中重试，避免负缓存与回源同时形成尖峰。
https://computing.example.test/catalog/13-negative-cache-jitter
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-negative-ttl | 45s | 新写入主动清除 |
| cod-miss-class | true negative | 区分冷启动缺失 |

## 14 标签失效与依赖图

标签失效适合批量更新 analyzer 或 field boost，依赖图必须无环。父标签变化触发子标签失效，消费者按拓扑深度处理。倒排索引重建与缓存失效可以使用同一标签，但二者提交状态需要分别记录。
https://computing.example.test/catalog/14-tag-invalidation-graph
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-tag-graph | acyclic | 拒绝循环依赖 |
| cod-dependency-depth | 3 | 按深度展开 |

## 15 预热与容量水位

预热按访问热度加载键，容量达到高水位时停止新加载。失效消息可以取消尚未完成的预载，回填结果必须带来源版本。排序算法用于选择预热集合时，应把预计收益与内存成本一起纳入 key。
https://computing.example.test/catalog/15-preload-capacity-watermark
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-high-watermark | 82% | 达到后停止加载 |
| cod-preload-version | 134 | 过期预载可取消 |

## 16 灰度回滚与指标核验

灰度只扩大一个变量，不同时修改缓存 TTL、analyzer 与比较器。回滚依据代际、命中率和端到端延迟，而不是单一缓存命中率。网络协议重试次数突然上升时，应先区分源站变慢与连接复用失效。
https://computing.example.test/catalog/16-canary-rollback-metrics
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-canary-scope | 10% | 单变量灰度 |
| cod-rollback-signal | generation lag | 结合延迟判断 |

## 17 帧边界与长度前缀

network protocol 使用长度前缀解决 TCP 粘包，maximum frame size 防止异常长度触发超大分配。接收端先校验 payload length 与 checksum，再反序列化。排序算法处理分片前，应按 sequence 恢复原始顺序。
https://computing.example.test/catalog/17-frame-length-prefix
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-max-frame-size | 65536 | 超限立即拒绝 |
| cod-checksum | crc32c | 解析前验证 |

## 18 握手与版本协商

握手交换 protocol version、能力集合与最大帧长，服务端选择双方支持的版本。未知能力必须被忽略或拒绝，不能误解析。倒排索引字段扩展也应版本化，旧客户端不应收到无法识别的新必填字段。
https://computing.example.test/catalog/18-handshake-version-negotiation
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-protocol-version | 3 | 双方兼容 |
| cod-capability-set | checksum,flow-control | 明确协商 |

## 19 超时预算与重试

connection timeout、read timeout 与总 deadline 分别计量，所有 retry 共享总预算。只有可重试错误才退避重试，参数错误立即返回。缓存失效期间的慢回源不能被无限重试放大，否则请求合并也会失去意义。
https://computing.example.test/catalog/19-timeout-retry-budget
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-total-deadline | 12s | 覆盖全部尝试 |
| cod-retry-limit | 2 | 仅可重试错误 |

## 20 幂等键与去重窗口

写请求携带 cod-idempotency-key，服务端保存请求摘要与响应摘要，在去重窗口内返回首次结果。相同键不同摘要必须冲突。缓存失效消息也使用幂等删除，但去重窗口应与协议层分开计量。
https://computing.example.test/catalog/20-idempotency-dedup-window
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-idempotency-key | req-8821 | 重试保持不变 |
| cod-dedup-window | 24h | 摘要冲突拒绝 |

## 21 拥塞窗口与退避

发送方维护拥塞窗口，超时后缩小窗口并执行 exponential backoff with jitter。乱序与丢包需要区分，不能把所有延迟都当作拥塞。排序算法的基准测试应单独记录网络等待，避免把协议成本计入比较次数。
https://computing.example.test/catalog/21-congestion-backoff
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-congestion-window | 64KiB | 超时后缩小 |
| cod-backoff | exponential jitter | 避免同步重试 |

## 22 流控与背压

window update 通知发送方可继续传输的字节数，应用 buffer 高水位暂停读取，低水位恢复。零窗口需要探测，避免双方永久等待。缓存回源消费速度不足时，也应把背压传递给请求合并队列。
https://computing.example.test/catalog/22-flow-control-backpressure
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-high-watermark | 80% | 暂停读取 |
| cod-low-watermark | 40% | 恢复读取 |

## 23 分片序号与重组

每个 stream 独立维护 sequence，接收端检测空洞并拒绝不完整消息。重组按序号排序，重复分片去重，超时释放 buffer。倒排链表的 position 也要求有序，但网络 sequence 的起点不必与字段位置相同。
https://computing.example.test/catalog/23-fragment-sequence-reassembly
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-stream-id | stream-17 | 序号在流内连续 |
| cod-reassembly-deadline | 5s | 超时释放 |

## 24 证书轮换与连接复用

证书轮换不应中断已协商的连接，新连接使用新证书并记录版本。连接复用必须保持 stream 状态隔离，request id 不能因换连接而改变语义。缓存键也不应绑定短期连接编号，否则复用会降低命中率。
https://computing.example.test/catalog/24-certificate-connection-reuse
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-certificate-version | 2026.11 | 新连接生效 |
| cod-connection-reuse | enabled | 流状态隔离 |

## 25 比较器全序

sorting algorithm 的比较器先按 score 降序，再按 document id 升序，最后按 version 降序。任意两个不同对象都必须分出大小。若比较器返回零，stable sort 只能保留输入顺序，不能自动消除并发段合并带来的波动。
https://computing.example.test/catalog/25-comparator-total-order
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-sort-key | score desc,document id asc | 全序且无并列 |
| cod-version-tiebreak | version desc | 最终决胜 |

## 26 稳定归并

stable merge 在相等 key 下保留左半区优先顺序，适合链表和外部排序。额外空间与元素数量相关，必须纳入内存预算。网络协议分片重组也使用归并思想，但需要额外检测 sequence 空洞。
https://computing.example.test/catalog/26-stable-merge
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-stability | guaranteed | 相等键保持输入序 |
| cod-extra-space | linear | 不超过内存预算 |

## 27 快速排序三路划分

Dutch national flag 把重复 score 的元素分为小于、等于、大于三区，减少无效交换。递归深度需要上限，退化输入可切换为堆排序。倒排索引候选常常同分，因此应专门测量三路划分的收益。
https://computing.example.test/catalog/27-three-way-quicksort
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-partition | three-way | 等值集中处理 |
| cod-recursion-limit | 32 | 超限切换算法 |

## 28 堆排序与 TopK

固定大小为 K 的堆维护当前最优候选，新候选只在超过堆顶时替换。堆排序全量原地排序的时间复杂度稳定，但常数开销可能高于归并。缓存失效导致候选集合变化时，TopK 也必须重新验证边界元素。
https://computing.example.test/catalog/28-heap-sort-topk
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-topk | 200 | 堆大小固定 |
| cod-heap-type | min heap | 堆顶为当前最弱候选 |

## 29 外部排序与归并段

外部排序先产生多个有序 run，再按内存与打开文件上限分层归并。每次归并只读取各 run 的当前记录，胜者推进后写入输出。协议重传可能增加读取成本，因此磁盘指标与网络等待要分开记录。
https://computing.example.test/catalog/29-external-sort-merge-runs
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-run-count | 8 | 不超过单次归并上限 |
| cod-merge-passes | 2 | 分层降低文件句柄 |

## 30 基数排序与键编码

radix sort 适用于可逆、保序的定长编码，负数、前缀长度和尾随零需要专门处理。中文 UTF-8 字节不直接等于业务排序规则。缓存键若采用编码后的 sort key，必须保证不同编码不会产生碰撞。
https://computing.example.test/catalog/30-radix-sort-key-encoding
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-key-encoding | order-preserving | 编码保持全序 |
| cod-collision-check | none | 不同对象不碰撞 |

## 31 排序后校验与分页游标

排序完成后检查 key 单调、总数守恒、重复对象去重。深分页游标保存最后一条完整 sort key，不使用易变 offset。联合检索还要核对缓存 generation 与协议 sequence，确保候选、内容与顺序来自同一可解释状态。
https://computing.example.test/catalog/31-sort-validation-page-cursor
```

```
| cod-dimension | cod-value | cod-check |
| --- | --- | --- |
| cod-page-cursor | last full sort key | 并发写入不重复已读键 |
| cod-final-state | ready | 四主题字段齐全 |
