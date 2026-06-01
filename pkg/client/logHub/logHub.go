package logHub

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"tacacs/pkg/public/log"
	"tacacs/pkg/public/loghub"
	"tacacs/pkg/public/tacplus"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// 异步上报基础参数。本地 clog 永远是 source of truth,这里所有失败/丢弃都不影响主路径。
const (
	// defaultSoftMaxQueueSize 是异步队列的默认软上限,只作为 OOM 防线;
	// 正常负载下 worker 持续抽走,len(buf) 长期接近 0,不会触发丢弃。
	defaultSoftMaxQueueSize = 100000
	maxBulkSize             = 300             // 单次 Bulk 最多打包条数(服务端硬上限 500,留余量)
	workerWriteTimeout      = 3 * time.Second // 单次 gRPC Bulk 超时,避免 worker 被卡死
	workerRetryMax          = 2               // 整批 RPC 失败时的额外重试次数(总尝试 = 1 + workerRetryMax)
	metricLogInterval       = 60 * time.Second
	flushTimeoutDefault     = 5 * time.Second // Stop 时等队列排空的默认超时
)

var (
	LogHub loghub.LogHubServiceClient
	MyApp  string
	// Enable 用 atomic.Bool: Init 写 true,Stop 写 false;enqueue 锁外做 fast-path 短路检查。
	Enable atomic.Bool

	// 异步队列:可增长 slice + sync.Cond。区别于 buffered channel:
	//   1. 没有 close 语义,Stop 不会与 enqueue 抢资源,代码上消掉了"send on closed chan"的窗口;
	//   2. 软上限只在异常堆积(下游不可达+重试失败)时触发丢弃,正常吞吐下一条都不丢;
	//   3. 单条入队仅持锁做 append + Signal,极短,不影响 tac+ 主路径。
	buf          []*loghub.LogEvent
	bufMu        sync.Mutex
	bufCond      *sync.Cond
	bufClosed    bool
	softMaxQueue int

	workerWG sync.WaitGroup
	stopOnce sync.Once

	// 异步路径上的可观测计数器,仅靠 metricLoop 周期输出,避免热路径 log 爆炸。
	metricEnqueued    atomic.Int64 // 成功入队的条数
	metricSent        atomic.Int64 // 真正写到 log-hub 成功的条数(按 server 返回的 successCount 累计)
	metricDroppedFull atomic.Int64 // 队列超过软上限被丢弃的条数(代码层 OOM 防线触发,正常负载下应恒为 0)
	metricFailedFinal atomic.Int64 // RPC 重试用完失败 + server 返回 failure 的条数
	metricWorkerPanic atomic.Int64 // worker 单次迭代 panic 后被 recover 重启的次数(>0 表示存在偶发 bug,应排查)

	metricStop chan struct{}
)

// Init 建立 gRPC client、启动单 worker + metric 协程。
// 失败时 Enable 保持 false,三个 LogXxx 入口会自动 short-circuit;
// 成功后 Enable=true,后续上报全部走异步队列,绝不阻塞主路径。
// queueSize <=0 时使用 defaultSoftMaxQueueSize;它仅是 OOM 防线,正常负载下不会触发丢弃。
func Init(grpcTarget, myApp string, queueSize int) error {
	if queueSize <= 0 {
		queueSize = defaultSoftMaxQueueSize
	}
	conn, err := grpc.NewClient(grpcTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	LogHub = loghub.NewLogHubServiceClient(conn)
	MyApp = myApp
	softMaxQueue = queueSize
	buf = make([]*loghub.LogEvent, 0, maxBulkSize)
	bufCond = sync.NewCond(&bufMu)
	bufClosed = false
	metricStop = make(chan struct{})
	Enable.Store(true)

	workerWG.Add(1)
	go runWorker()
	go runMetricLoop(metricLogInterval)
	return nil
}

// Stop 关闭入队、等 worker 把残余队列写完(或超时强退)。
// 主进程优雅退出时调用,保证最后一批记录不丢。
// 重复调用安全。timeout <=0 走 flushTimeoutDefault。
func Stop(timeout time.Duration) {
	if !Enable.Load() {
		return
	}
	stopOnce.Do(func() {
		if timeout <= 0 {
			timeout = flushTimeoutDefault
		}
		// 翻 Enable → 新 enqueue 立刻短路;
		// 设 closed=true + Broadcast → 唤醒 worker 把残余 flush 完后退出。
		Enable.Store(false)
		bufMu.Lock()
		bufClosed = true
		bufCond.Broadcast()
		bufMu.Unlock()

		done := make(chan struct{})
		go func() {
			workerWG.Wait()
			close(done)
		}()
		select {
		case <-done:
			log.Logger.Infof("logHub stopped: enqueued=%d sent=%d droppedFull=%d failedFinal=%d workerPanic=%d",
				metricEnqueued.Load(), metricSent.Load(), metricDroppedFull.Load(), metricFailedFinal.Load(), metricWorkerPanic.Load())
		case <-time.After(timeout):
			bufMu.Lock()
			remaining := len(buf)
			bufMu.Unlock()
			log.Logger.Errorf("logHub flush timeout after %v: enqueued=%d sent=%d droppedFull=%d failedFinal=%d workerPanic=%d remaining=%d",
				timeout, metricEnqueued.Load(), metricSent.Load(), metricDroppedFull.Load(), metricFailedFinal.Load(), metricWorkerPanic.Load(), remaining)
		}
		close(metricStop)
	})
}

// runWorker 攒批消费 buf:
//   - bufCond.Wait 等到 buf 非空 → 一次性取最多 maxBulkSize 条 → Bulk 发出。
//   - bufClosed=true 且 buf 已空 → 退出。
//   - 单次迭代 panic 不会让 worker 死: workerIteration 内 recover 后由 runWorker 立刻
//     起下一轮,防止某个 batch 触发的偶发 bug 把 logHub 旁路通道悄悄停摆。
//
// 单 worker 设计:gRPC IO bound,顺序写避免连接竞争与日志乱序,也便于运维理解。
func runWorker() {
	defer workerWG.Done()
	for {
		if workerIteration() {
			return
		}
	}
}

// workerIteration 跑一次"取批 + 发批"。返回 true 表示 worker 应当退出(Stop 已下发且队列排空)。
// 单次 iteration 的所有 panic 都在本函数内被 recover,worker 不退出 —— 只累一笔
// metricWorkerPanic 然后由 runWorker 重新进入下一轮。
// Enable=true 时短暂 sleep 防止确定性 panic 把 CPU 打满 + 日志炸掉;
// Enable=false 表明 Stop 已下发,不 sleep,让出 CPU 后下次 pullBatch 立即看到 bufClosed 退出,
// 不让 panic 重启路径吃掉 Stop 的 5s flush 窗口。
func workerIteration() (shutdown bool) {
	defer func() {
		if r := recover(); r != nil {
			metricWorkerPanic.Add(1)
			log.Logger.Errorf("logHub worker panic recovered: %v\n%s", r, debug.Stack())
			if Enable.Load() {
				time.Sleep(time.Second)
			}
		}
	}()
	batch, done := pullBatch()
	if done {
		return true
	}
	sendBulk(batch)
	return false
}

// pullBatch 拿锁等待 / 抽出最多 maxBulkSize 条事件,bufMu 用 defer 保证释放,
// 这样即便锁内代码 panic,也不会泄漏锁。
// done=true 表示队列已关闭且为空,worker 应退出。
func pullBatch() (batch []*loghub.LogEvent, done bool) {
	bufMu.Lock()
	defer bufMu.Unlock()
	for len(buf) == 0 && !bufClosed {
		bufCond.Wait()
	}
	if len(buf) == 0 && bufClosed {
		return nil, true
	}
	n := len(buf)
	if n > maxBulkSize {
		n = maxBulkSize
	}
	// 拷出独立 batch 后把剩余事件左移并 nil 化尾部:
	//   - batch 中的事件指针不再被 buf 底层数组持有,sendBulk 期间这部分内存可被独立 GC;
	//   - 锁通过 defer 在函数返回时释放,生产者继续 append 不被阻塞。
	batch = make([]*loghub.LogEvent, n)
	copy(batch, buf[:n])
	copy(buf, buf[n:])
	tail := len(buf) - n
	for i := tail; i < len(buf); i++ {
		buf[i] = nil
	}
	buf = buf[:tail]
	return batch, false
}

// sendBulk 发一批 Bulk,处理"整批 RPC 失败"与"server 返回部分失败"两种情况。
// 部分失败不重试:重投会让原本成功的项重复写,而 BulkItemResult 没有原 event 指针可定位失败项。
// 错误日志统一带上 server 回填的 x-trace-id,方便去 log-hub 侧反查这条上报到底卡在哪。
func sendBulk(events []*loghub.LogEvent) {
	if len(events) == 0 {
		return
	}
	req := &loghub.BulkRequest{Events: events}
	var lastErr error
	var lastTraceID string
	for attempt := 0; attempt <= workerRetryMax; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), workerWriteTimeout)
		ctx = metadata.AppendToOutgoingContext(ctx, "x-loghub-caller", MyApp)
		// 每次 attempt 独立的 hdr,避免重试间污染拿到上一次的 trace。
		var hdr metadata.MD
		resp, err := LogHub.Bulk(ctx, req, grpc.Header(&hdr))
		cancel()
		traceID := firstMDValue(hdr, "x-trace-id")
		if err == nil {
			success := int64(resp.GetSuccessCount())
			failure := int64(resp.GetFailureCount())
			// 兜底:如果 server 没填 successCount/failureCount,从 items 重算;
			// items 也空时,乐观认为整批成功(RPC OK 通常意味着 server 已接收落盘)。
			if success == 0 && failure == 0 {
				if items := resp.GetItems(); len(items) > 0 {
					for _, it := range items {
						if it.GetSuccess() {
							success++
						} else {
							failure++
						}
					}
				} else {
					success = int64(len(events))
				}
			}
			metricSent.Add(success)
			if failure > 0 {
				metricFailedFinal.Add(failure)
				log.Logger.Errorf("logHub bulk partial failure: batch=%d success=%d failure=%d trace=%s sample=%s",
					len(events), success, failure, traceID, sampleFirstBulkError(resp.GetItems()))
			}
			return
		}
		lastErr = err
		lastTraceID = traceID
		if attempt < workerRetryMax {
			// 简单线性退避: 100ms, 200ms。worker 单线程,不必激进。
			time.Sleep(time.Duration(100*(attempt+1)) * time.Millisecond)
		}
	}
	// 整批 RPC 完全失败:这一批全部计为 failedFinal。
	metricFailedFinal.Add(int64(len(events)))
	log.Logger.Errorf("logHub bulk write failed after %d retries, batch=%d trace=%s err=%v",
		workerRetryMax, len(events), lastTraceID, lastErr)
}

func firstMDValue(md metadata.MD, key string) string {
	if vs := md.Get(key); len(vs) > 0 {
		return vs[0]
	}
	return ""
}

func sampleFirstBulkError(items []*loghub.BulkItemResult) string {
	for _, it := range items {
		if it.GetSuccess() {
			continue
		}
		e := it.GetError()
		if e == nil {
			return fmt.Sprintf("stream=%s (no error detail)", it.GetStream())
		}
		return fmt.Sprintf("stream=%s code=%s msg=%s field=%s", it.GetStream(), e.GetCode(), e.GetMessage(), e.GetField())
	}
	return ""
}

// runMetricLoop 周期性把 5 个计数器打到 app.log,运维一眼能看到是否在丢 / 是否有 panic 残留。
func runMetricLoop(interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	var lastSent, lastDropped, lastFailed, lastPanic int64
	for {
		select {
		case <-t.C:
			sent := metricSent.Load()
			dropped := metricDroppedFull.Load()
			failed := metricFailedFinal.Load()
			panicN := metricWorkerPanic.Load()
			bufMu.Lock()
			queued := len(buf)
			bufMu.Unlock()
			log.Logger.Infof("logHub metrics: queued=%d enqueued=%d sent=%d (+%d) droppedFull=%d (+%d) failedFinal=%d (+%d) workerPanic=%d (+%d)",
				queued, metricEnqueued.Load(),
				sent, sent-lastSent,
				dropped, dropped-lastDropped,
				failed, failed-lastFailed,
				panicN, panicN-lastPanic)
			lastSent, lastDropped, lastFailed, lastPanic = sent, dropped, failed, panicN
		case <-metricStop:
			return
		}
	}
}

// enqueue 非阻塞入队。Enable=false 立即 short-circuit;
// 软上限只在异常堆积(下游不可达 + 重试失败)时触发丢弃,作为代码层 OOM 防线。
// 正常负载下 worker 持续抽走,len(buf) 长期接近 0,这条丢弃路径形同虚设。
func enqueue(event *loghub.LogEvent) {
	if !Enable.Load() {
		return
	}
	bufMu.Lock()
	if bufClosed {
		bufMu.Unlock()
		return
	}
	if len(buf) >= softMaxQueue {
		bufMu.Unlock()
		metricDroppedFull.Add(1)
		return
	}
	buf = append(buf, event)
	bufCond.Signal()
	bufMu.Unlock()
	metricEnqueued.Add(1)
}

func strAttr(s string) *loghub.AttributeValue {
	return &loghub.AttributeValue{Value: &loghub.AttributeValue_StringValue{StringValue: s}}
}
func intAttr(i int64) *loghub.AttributeValue {
	return &loghub.AttributeValue{Value: &loghub.AttributeValue_IntValue{IntValue: i}}
}
func boolAttr(b bool) *loghub.AttributeValue {
	return &loghub.AttributeValue{Value: &loghub.AttributeValue_BoolValue{BoolValue: b}}
}
func strArrayAttr(ss []string) *loghub.AttributeValue {
	values := make([]*loghub.AttributeValue, 0, len(ss))
	for _, s := range ss {
		values = append(values, strAttr(s))
	}
	return &loghub.AttributeValue{Value: &loghub.AttributeValue_ArrayValue{ArrayValue: &loghub.AttributeArray{Values: values}}}
}

// LogAccount / LogAuthor / LogAuthen 是异步入口:构建 event 后非阻塞入队即返回。
// ctx 形参保留以维持调用方签名,但内部不使用 —— 异步路径不能依赖调用方 ctx 的生命周期。
func LogAccount(_ context.Context, info *tacplus.AccountInfo) {
	enqueue(&loghub.LogEvent{
		Stream:            "tacacs_account",
		TimestampUnixNano: info.TimeStamp,
		SeverityText:      "INFO",
		Attributes: map[string]*loghub.AttributeValue{
			"time":            strAttr(info.Time),
			"timeStamp":       intAttr(info.TimeStamp),
			"timeRange":       intAttr(info.TimeRange),
			"user":            strAttr(info.User),
			"switchAddr":      strAttr(info.SwitchAddr),
			"serverAddr":      strAttr(info.ServerAddr),
			"cmd":             strAttr(info.Cmd),
			"port":            strAttr(info.Port),
			"flags":           intAttr(int64(info.Flags)),
			"authenMethod":    intAttr(int64(info.AuthenMethod)),
			"privLvl":         intAttr(int64(info.PrivLvl)),
			"authenType":      intAttr(int64(info.AuthenType)),
			"authenService":   intAttr(int64(info.AuthenService)),
			"arg":             strArrayAttr(info.Arg),
			"isSingleConnect": boolAttr(info.IsSingleConnect),
			"tacacsClient":    strAttr(info.TacacsClient),
		},
	})
}

func LogAuthor(_ context.Context, info *tacplus.AuthorInfo) {
	enqueue(&loghub.LogEvent{
		Stream:            "tacacs_author",
		TimestampUnixNano: info.TimeStamp,
		SeverityText:      "INFO",
		Attributes: map[string]*loghub.AttributeValue{
			"time":            strAttr(info.Time),
			"timeStamp":       intAttr(info.TimeStamp),
			"timeRange":       intAttr(info.TimeRange),
			"user":            strAttr(info.User),
			"switchAddr":      strAttr(info.SwitchAddr),
			"serverAddr":      strAttr(info.ServerAddr),
			"authorStatus":    strAttr(info.AuthorStatus),
			"details":         strAttr(info.Details),
			"cmd":             strAttr(info.Cmd),
			"isSingleConnect": boolAttr(info.IsSingleConnect),
			"tacacsClient":    strAttr(info.TacacsClient),
		},
	})
}

func LogAuthen(_ context.Context, info *tacplus.AuthenInfo) {
	enqueue(&loghub.LogEvent{
		Stream:            "tacacs_authen",
		TimestampUnixNano: info.TimeStamp,
		SeverityText:      "INFO",
		Attributes: map[string]*loghub.AttributeValue{
			"time":            strAttr(info.Time),
			"timeStamp":       intAttr(info.TimeStamp),
			"timeRange":       intAttr(info.TimeRange),
			"user":            strAttr(info.User),
			"switchAddr":      strAttr(info.SwitchAddr),
			"serverAddr":      strAttr(info.ServerAddr),
			"authenStatus":    strAttr(info.AuthenStatus),
			"details":         strAttr(info.Details),
			"isSingleConnect": boolAttr(info.IsSingleConnect),
			"tacacsClient":    strAttr(info.TacacsClient),
		},
	})
}
