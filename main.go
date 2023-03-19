package main

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"time"
)

// TODO 1.日志的处理 2.命令行参数(用go-flags库) 3.也用协程实现一个col中有多个Stream
// 但是现在暂时是做完了

// SyncSlice 包装了[]*Stream,其中sync.Mutex用于控制colStreams在数组扩容时不会发生内存泄露
type SyncSlice struct {
	mutex      sync.Mutex // 信号量中的互斥量,mutex
	colStreams []*Stream
}

// Stream 用于控制每列的流
type Stream struct {
	col      int  // 列号
	length   int  // 流的长度
	headPos  int  // 头部的位置
	headDone bool // 判断头部是否到底(这个参数没有也行,只是为了编程方便,即不需要再判断headPos是否大于height了,省的只判断headPos的值)
	tailPos  int  // 尾部的位置
	exist    bool // 是否存在这个流
}

// Size 窗口的宽度和长度,由于tcell这个库是width在前,所以是width,height,而不是height,width
type Size struct {
	width  int
	height int
}

func (s *Size) setSize(w, h int) {
	s.width = w
	s.height = h
}

func main() {

	provideSpeed := 100     // 产生雨的速度
	downSpeed := 50         // 雨下落的速度
	baseLength := 15        // 雨的长度
	randomLengthRange := 15 // 雨的变化范围

	// defStyle为全为背景和字符全为黑色
	defStyle := tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset) // 设置颜色,用于初始化和刷新终端

	// 背景色为黑色,字符为绿色
	greenStyle := tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorGreen)

	// 当前窗口的大小,用于当窗口大小变化时,存储变化后的大小
	var curSize Size

	// 设置代码雨中用于填充的字符集
	charSet := []rune{'0', '1'}

	// tcell screen对象的初始化
	screen, err := tcell.NewScreen()
	if err != nil {
		log.Fatalf("%+v", err) // log.Fatalf函数会打印日志后调用os.Exit(1)函数
	}
	err = screen.Init()
	if err != nil {
		log.Fatalf("%+v", err)
	}

	// 隐藏cursor(光标)
	screen.HideCursor()

	screen.SetStyle(defStyle)
	// 清屏
	screen.Clear()

	screen.SetStyle(greenStyle)

	// 获取当前窗口大小
	w, h := screen.Size()

	// 设置当前窗口大小
	curSize.setSize(w, h)

	sync := SyncSlice{
		// 创建colStreams的slice,用于存储Stream的指针
		colStreams: make([]*Stream, curSize.width),
	}

	// 调用os的系统调用,捕获sig(signal)信号(比如sigkill)
	sigChan := make(chan os.Signal, 1)            // 创建一个os.Signal类型的通道,其中buffer的值为1
	signal.Notify(sigChan, os.Interrupt, os.Kill) // 调用os/signal中的Notify方法为之前创建的sigChan通道进行注册,如果接收到对应注册的信号就有值

	// 随机种子
	rand.Seed(time.Now().UnixNano())

	// 初始化colStreams切片,exist为false表示为不存在
	for i := 0; i < curSize.width; i++ {
		sync.colStreams[i] = &Stream{
			exist: false,
		}
	}

	// 创建用于更新的channel sizeUpdateCh
	// 当触发了tcell中的ReSize事件时,向sizeUpdateCh发送信号,让协程开始工作(改为调用函数来更新也是可以的)
	sizeUpdateCh := make(chan Size)

	// 可以考虑一开始就开一个很大的数组,而不是动态分配内存,这样就没有内存越界的问题了
	// 但是我还是想试试怎么来动态分配内存,先写着吧

	// slice动态分配内存
	// 当slice的底层容量(cap)不够用的时候,会重新分配内存空间并将老的内容复制过来
	// 参考文章
	// https://blog.go-zh.org/go-slices-usage-and-internals

	// 此协程用于当ReSize事件触发时,将slice进行切片或者进行扩容
	go func() {

		// 上一次的宽度
		lastWidth := curSize.width

		// 这里用for range来遍历一个channel,当channel中收到数据的时候,才执行,否则一直阻塞(有缓冲和无缓冲都可以用,效果都是等有数据进来就执行,没有就阻塞,有缓冲的相当于多了一个等待队列)
		// 可以用for range,也可以用其他形式,比如select表达式来做,效果是一样的
		for newSize := range sizeUpdateCh {

			// 判断宽度时候变化
			diffWidth := newSize.width - lastWidth

			// 宽度没变,不需要对slice进行调整
			if diffWidth == 0 {
				continue
			}

			// 宽度增加,进行扩容
			if diffWidth > 0 {
				// 加锁,防止在扩容的时候操作切片,不加锁可能导致内存泄漏(当然在低并发的条件下看不出来会泄漏,但是还是要加锁)
				sync.mutex.Lock()
				//log.Println("before allocate")

				// go中的struct有默认值
				// 创建要添加的Stream
				newColStreams := make([]*Stream, diffWidth)
				for i := 0; i < diffWidth; i++ {
					newColStreams[i] = &Stream{
						exist: false,
					}
				}
				// 进行扩容
				sync.colStreams = append(sync.colStreams, newColStreams...) // 针对slice的合并,需要用...语法来解包unpack
				// 扩容完毕,释放锁
				sync.mutex.Unlock()
				//log.Println("after allocate")
			}

			// 如果窗口缩小了,进行slice
			if diffWidth < 0 {
				sync.mutex.Lock()
				// 进行切片操作
				sync.colStreams = sync.colStreams[0:newSize.width]
				sync.mutex.Unlock()
			}

		}

	}()

	// 创建tcell.Event类型的channel,当有tcell事件到来时,event将被发送到eventChan中,没有事件到来时,阻塞在screen.PollEvent()中
	eventChan := make(chan tcell.Event)
	go func() {
		for {
			event := screen.PollEvent()
			eventChan <- event
		}
	}()

	// 开启一个goroutine,用于向colStreams中添加Stream,其中如果流已经存在,那么就再重新随机,其中要用锁变量(sync.Mutex)互斥量进行同步
	go func() {
		for {

			time.Sleep(time.Millisecond * time.Duration(provideSpeed))

			randomInt := rand.Intn(curSize.width)

			//log.Println("generate random number:", randomInt)

			sync.mutex.Lock()
			if sync.colStreams[randomInt].exist {

			} else {
				sync.colStreams[randomInt] = &Stream{
					col:      randomInt,
					length:   baseLength + rand.Intn(randomLengthRange), // 随机长度
					headPos:  0,
					headDone: false,
					tailPos:  0,
					exist:    true,
				}
			}
			sync.mutex.Unlock()
		}
	}()

	// 开启一个goroutine,用于让雨滴下落
	// 原理为模拟一个队列结构,储存headPos和tailPos
	// 其中headPos在每次刷新中自增,用SetContent和随机出的rune来填充这个值
	// 等headPos到底部了之后,设置headDone为true,不再刷新
	// tailPos的自增要等headPos的值大于length之后才开始
	// 将tailPos处的字符用SetContent函数置为' '空格这个空字符
	// 当tailPos到底部了之后,说明Stream已经结束了,将exist的值置为false,表示可以开启新的Stream
	//
	// 显示出下降的原理就是增加头部,中间不动,删去尾部,和我自己之前写的贪吃蛇运动的原理是一样的
	//
	// 这种用exist的方法实现起来比较简单,但是有个缺点就是一列同时只能存在一个Stream,不能有多个Stream
	// 但是我所参考的gomatrix是可以在一列中有多个Stream的,其原理是用协程来进行管理,有一个StreamDisplay,
	// StreamDisplay能对应n个Stream,用channel来进行信号的通知,一个Stream被一个StreamDisplay持有,
	// 一个StreamDisplay中持有n个Stream,最开始时初始化一个Stream,等到tailPos>0(表示已经全部出来了)
	// 向StreamDisplay中的channel发送信号,StreamDisplay再进行创建新的Stream,要停止协程时,
	// 也是向Stream中控制停止的channel发送信号就可以停止,由于每个Stream是运行在
	// 一个单独的协程中的,不需要一个集中的manager来管理,只需要管理StreamDisplay就可以了,StreamDisplay能做到自动刷新界面
	// 所以能做到一列中有多个Stream,但是Stream因为速度不一样的原因,有重叠的可能性
	//
	// 由于我懒,所以只做了一个简单版本的,有兴趣的直接看gomatrix的源码吧,用channel用的很多
	go func() {
		for {

			// screen.Show()中有锁,保证原子刷新
			screen.Show()
			//log.Println("render")

			time.Sleep(time.Millisecond * time.Duration(downSpeed))
			sync.mutex.Lock()
			// curSize的值是动态变化的,每次会执行一个新的for循环
			for i := 0; i < curSize.width; i++ {

				stream := sync.colStreams[i]

				// 判读是否存在Stream
				if stream.exist {

					// 对头部的处理
					if !stream.headDone && stream.headPos <= curSize.height {
						newRune := charSet[rand.Intn(2)]
						screen.SetContent(stream.col, stream.headPos, newRune, nil, greenStyle)
						stream.headPos++
					} else {
						stream.headDone = true
					}

					// 对尾部的处理
					if stream.tailPos > 0 || stream.headPos >= stream.length {

						if stream.tailPos < curSize.height {
							screen.SetContent(stream.col, stream.tailPos, ' ', nil, defStyle)
							stream.tailPos++
						} else {
							stream.exist = false
						}

					}
				}
			}
			sync.mutex.Unlock()
		}
	}()

	// 主线程中的事件循环
	// EVENTS是label标号,用于跳出外层循环的
EVENTS:
	for {
		select {
		// tcell的事件
		case event := <-eventChan:
			// switch来判断事件的类型(和类型断言有点像)
			switch ev := event.(type) {
			case *tcell.EventKey:
				switch ev.Key() {
				// ctrl c的话,退出
				case tcell.KeyCtrlC:
					break EVENTS
				}

			// 如果是ReSize事件的话,向sizeUpdateCh发送cursize,进行更新
			case *tcell.EventResize:
				w, h := ev.Size()
				curSize.setSize(w, h)
				sizeUpdateCh <- curSize // 这里用函数来做其实也可以,goroutine模型是用来做并发的,cpu密集型任务不能提高效率
			// 报错处理
			case *tcell.EventError:
				log.Println(ev.Error())
			}
		// 操作系统的signal事件
		case <-sigChan:
			// 结束循环
			break EVENTS
		}
	}

	//程序结束了,回收资源
	screen.Fini()

	fmt.Println("The matrix is over")

}
