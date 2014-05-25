package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"gotest/greentea/serial"
	"gotest/greentea/tools"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

type configObj struct {
	ComName       string        // 串口名称
	Baud          int           // 波特率
	IniSavePath   string        // ini文件保存路径
	BmpSavePath   string        // bmp保存路径
	LogSavePath   string        // log文件记录
	LogDaysToKeep time.Duration // log保存天数
	BmpDaysToKeep time.Duration // bmp保存天数

}

const (
	DATA_S_FLAG, DATA_E_FLAG     = "*s[start]s*", "*s[output_end]s*" // 本笔结束
	MSG_S_FLAG, MSG_E_FLAG       = "*s[", "]s*"                      // 机器信息标识位
	I_S_DATE_FLAG, I_E_DATE_FLAG = "*d{", "}d*"                      // 数据日期标识位
	I_S_TIME_FLAG, I_E_TIME_FLAG = "*t{", "}t*"                      // 数据时间标识位
	I_S_NO_FLAG, I_E_NO_FLAG     = "*no{", "}no*"                    // 数据顺序号标识位
	I_S_BN_FLAG, I_E_BN_FLAG     = "*bn{", "}bn*"                    // 数据冠字号标识位
	M_S_DATE_FLAG, M_E_DATE_FLAG = "*d[", "]d*"                      // 机器状态：数据日期标识位
	M_S_TIME_FLAG, M_E_TIME_FLAG = "*t[", "]t*"                      // 机器状态：数据时间标识位

	STATUS_INIT      int = 1 // 初始化工作
	STATUS_READ_DONE int = 2 // 读取完成

	LineBreak = "\r\n" // windows 换行
)

var (
	com           io.ReadWriteCloser       // 串口对象
	buffer        = make([]byte, 0, 6<<10) // 缓冲区
	ok            chan int                 // 信号量
	readable      chan byte                //读信号量
	config        *configObj               // 配置文件
	countTimesDay int                      // 当天交易次数
	currentDay    string                   // 今天日期
	snrinfo       *os.File                 // ini文件对象
	snrlog        *os.File                 // 日志记录
	bmpPath       string                   // bmp 路径
	bmpFile       *os.File                 // bmp对象
	err           error                    // 全局err对象
	logBuffer     []byte                   // log临时缓冲区

)

func init() {

	pwd, _ := os.Getwd()
	pwd = filepath.Join(pwd, "config.json")

	config = new(configObj)
	file, e := ioutil.ReadFile(pwd)
	if e != nil {
		fmt.Println("读取配置文件失败!请与管理员联系!")
		os.Exit(1)
	}
	json.Unmarshal(file, config)

	connectCom()

	// 检查当前最后笔数
	now := time.Now().Format("20060102")
	currentDay = now
	path := filepath.Join(config.BmpSavePath, now)
	files, err := ioutil.ReadDir(path)
	if err == nil {
		fmt.Println("times:", len(files))
		countTimesDay = len(files)
	}

	// 删除文件
	os.Remove(config.IniSavePath)
}

func connectCom() {
	c := &serial.Config{Name: config.ComName, Baud: config.Baud}
	com, err = serial.OpenPort(c)
	if err != nil {
		fmt.Println("打开串口", config.ComName, "失败:")
		fmt.Println("3秒后重新连接!")
		time.Sleep(3 * time.Second)
		connectCom()
	}
	fmt.Println("串口:", config.ComName, "连接成功！")
	err = nil
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	ok = make(chan int)
	readable = make(chan byte, 1)
	exit := make(chan bool)
	bmpClearTask := tools.NewTask("bmpClearTask", "59 59 23 * * * ", bmpClear)
	logClearTask := tools.NewTask("logClearTask", "59 59 01 * * * ", logClear)
	tools.AddTask("bmpClearTask", bmpClearTask)
	tools.AddTask("logClearTask", logClearTask)
	tools.StartTask()
	defer tools.StopTask()

	go read()

	// go func() {
	// 	for {
	// 		select {
	// 		case <-time.After(5 * time.Second):
	// 			fmt.Printf("len:%d cap:%d pointer:%p\n", len(buffer), cap(buffer), buffer)

	// 		}
	// 	}
	// }()
	go parse()

	<-exit
}

func read() {
	var inbyte = make([]byte, 1024)
	var n int
	for {
		if bytes.Contains(buffer, []byte(MSG_E_FLAG)) {
			ok <- STATUS_READ_DONE
			//parse()
		}
		n, err = com.Read(inbyte)
		if err != nil {
			fmt.Println("读取串口信息失败：请检查连接！")
			connectCom()
			continue
		}
		err = nil
		buffer = append(buffer, inbyte[0:n]...)
		//}

	}
}

func parse() {
	for {
		select {
		case <-ok:
			now := time.Now().Format("20060102")
			if now != currentDay { // 每日清空交易笔数
				countTimesDay = 0
				currentDay = now
			}

			// 创建日志目录
			err = os.MkdirAll(config.LogSavePath, 0666)
			if err != nil {
				fmt.Println("创建日志目录失败", err)
			}
			err = nil

			// ===============状态信息处理开始===============

			var n int

			// parse machine state
			// endIndex = bytes.Index(buffer, []byte(DATA_E_FLAG))
			// logBuffer = buffer[0 : endIndex+len(DATA_E_FLAG)]
			n = bytes.Count(buffer, []byte(MSG_E_FLAG))
			if n > 0 {
				// open or create a log file
				snrlog, err = os.OpenFile(filepath.Join(config.LogSavePath, now+".log"),
					os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
				if err != nil {
					fmt.Println("创建日志文件失败！", err)
				}
				err = nil

				fmt.Println("初始化成功。。。")
			}
			fmt.Println("log跑", n, "次")
			var startIndex int // 开始索引
			var endIndex int   // 结束索引
			for i := 0; i < n; i++ {
				startIndex = bytes.Index(buffer, []byte(M_S_DATE_FLAG))
				fmt.Println("startIndex:", startIndex)
				endIndex = bytes.Index(buffer, []byte(MSG_E_FLAG))
				fmt.Println("endIndex:", endIndex)
				snrlog.WriteString(string(buffer[startIndex:endIndex+len(MSG_E_FLAG)]) + LineBreak)
				//buffer = buffer[endIndex+len(MSG_E_FLAG):]
				// clear data
				buffer = bytes.Replace(buffer, buffer[startIndex:endIndex+len(MSG_E_FLAG)], []byte(""), 1)
			}
			if snrlog != nil {
				snrlog.Close()
			}
			// ===============状态信息处理结束===============

			// ===============信息数据处理开始===============
			// to start parse data
			n = bytes.Count(buffer, []byte(I_E_NO_FLAG))
			//o := bytes.Count(buffer, []byte(DATA_E_FLAG))
			if n > 0 {
				// create directory
				countTimesDay += 1 // 统计交易笔数
				ctd := strconv.Itoa(countTimesDay)
				path := filepath.Join(config.BmpSavePath, currentDay, ctd)
				err = os.MkdirAll(path, 0666)
				if err != nil {
					fmt.Println("创建BMP目录失败", err)
				}
				err = nil

				// open or create SNRinfo.ini
				snrinfo, err = os.OpenFile(config.IniSavePath, os.O_CREATE|os.O_WRONLY, 0666)
				if err != nil {
					fmt.Println("创建SNRinfo.ini文件失败！", err)
				}
				err = nil

				snrinfo.WriteString("[Cash_Info]" + LineBreak)
				snrinfo.WriteString("LEVEL4_COUNT=" + strconv.Itoa(n) + LineBreak)
				snrinfo.WriteString("LEVEL3_COUNT=0" + LineBreak)
				snrinfo.WriteString("LEVEL2_COUNT=0" + LineBreak)
				snrinfo.WriteString("OperationTime=" + time.Now().Format("2006-01-02 15:04:05") + LineBreak)
				snrinfo.WriteString(LineBreak)

				// parse info date
				i_date_s_index := bytes.Index(buffer, []byte(I_S_DATE_FLAG))
				i_time_e_index := bytes.Index(buffer, []byte(I_E_TIME_FLAG))
				buffer = bytes.Replace(buffer, buffer[i_date_s_index:i_time_e_index+len(I_E_TIME_FLAG)], []byte(""), 1)

				for i := 0; i < n; i++ {
					snrinfo.WriteString(LineBreak)
					si := strconv.Itoa(i + 1)
					nums := si
					lens := len(si)
					if lens == 1 {
						nums = "00" + si
					} else if lens == 2 {
						nums = "0" + si
					}
					snrinfo.WriteString("[LEVEL4_" + nums + "]" + LineBreak)
					snrinfo.WriteString("Index=" + strconv.Itoa(i) + LineBreak)
					snrinfo.WriteString("Value=100" + LineBreak)

					// parse info no
					i_no_s_index := bytes.Index(buffer, []byte(I_S_NO_FLAG))
					i_no_e_index := bytes.Index(buffer, []byte(I_E_NO_FLAG))
					i_no_data := buffer[i_no_s_index+len(I_S_NO_FLAG) : i_no_e_index]

					i_no_data_str := string(i_no_data) // 给后面使用

					bmpPath = filepath.Join(path, i_no_data_str+".bmp")

					// clear no data
					buffer = bytes.Replace(buffer, buffer[i_no_s_index:i_no_e_index+len(I_E_NO_FLAG)], []byte(""), 1)
					//buffer = buffer[i_no_e_index+len(I_E_NO_FLAG):]

					// parse info bn
					i_bn_s_index := bytes.Index(buffer, []byte(I_S_BN_FLAG))
					i_bn_e_index := bytes.Index(buffer, []byte(I_E_BN_FLAG))
					i_bn_data := buffer[i_bn_s_index+len(I_S_BN_FLAG) : i_bn_e_index]

					snrinfo.WriteString("SerialNumber=" + string(i_bn_data) + LineBreak)

					// clear bn data
					buffer = bytes.Replace(buffer, buffer[i_bn_s_index:i_bn_e_index], []byte(""), 1)
					//buffer = buffer[i_bn_e_index:]

					i_bmp_s_index := bytes.Index(buffer, []byte(I_E_BN_FLAG))
					var i_bmp_e_index int
					// parse info bmp
					if i < (n - 1) { // bmpEndFlag:= "*bn{02}bn*"
						num, _ := strconv.Atoi(string(i_no_data_str))
						num = num + 1
						numstr := strconv.Itoa(num)
						if num < 10 {
							numstr = "0" + numstr
						}
						i_bmp_e_index = bytes.Index(buffer, []byte(I_S_NO_FLAG+numstr+I_E_NO_FLAG))
					} else { // bmpEndFlag:= "*s[output_end]s*"
						//bmpEndFlag = M_S_DATE_FLAG
						i_bmp_e_index = len(buffer)
					}

					fmt.Println(">>>> ", i_bmp_s_index+len(I_E_BN_FLAG), i_bmp_e_index)
					i_bmp_data := buffer[i_bmp_s_index+len(I_E_BN_FLAG) : i_bmp_e_index]

					// write bmp to file
					snrinfo.WriteString("ImageFile=" + bmpPath + LineBreak)
					fmt.Println("i_bmp_data", len(i_bmp_data))
					bmpFile, err = os.OpenFile(bmpPath, os.O_CREATE|os.O_WRONLY, 0666)
					if err != nil {
						fmt.Println("创建bmp文件失败：" + bmpPath)
					}
					err = nil
					bmpFile.Write(i_bmp_data)

					if bmpFile != nil {
						bmpFile.Close()
					}
					bmpPath = ""
					bmpFile = nil

					// clear bmp data
					buffer = bytes.Replace(buffer, buffer[i_bmp_s_index:i_bmp_e_index], []byte(""), 1)
					//buffer = buffer[i_bmp_e_index:]
					// ===============信息数据处理结束===============
				}

				// 一笔结束，回收资源
				if snrinfo != nil {
					snrinfo.Close()
				}

			}

			//endIndex = bytes.Index(buffer, []byte(DATA_E_FLAG))
			//buffer = buffer[endIndex+len(DATA_E_FLAG):]
			//buffer = buffer[0:0]
			//logBuffer = logBuffer[0:0]
			//time.Sleep(30 * time.Second)
			fmt.Println("done...")

		case <-time.After(5 * time.Second):
			fmt.Printf("len:%d cap:%d\n", len(buffer), cap(buffer))
			fmt.Println("超时了")

		}
	}
}

func bmpClear() error {
	// 定时检查过期数据
	files, err := ioutil.ReadDir(config.BmpSavePath)
	if err != nil {
		return errors.New("未找到BMP目录：" + err.Error())
	}
	for _, file := range files {
		filename := file.Name()
		t, e := time.Parse("20060102", filename)
		if e != nil {
			fmt.Println("目录名称不正确", e)
			continue
		}
		if time.Now().Sub(t.Add(config.BmpDaysToKeep*24*time.Hour)) > 0 {
			os.RemoveAll(filepath.Join(config.BmpSavePath, filename))
		}
	}
	return nil
}

func logClear() error {
	// 定时检查过期数据
	files, err := ioutil.ReadDir(config.LogSavePath)
	if err != nil {
		return errors.New("未找到Log目录：" + err.Error())
	}
	for _, file := range files {
		filename := file.Name()
		t, e := time.Parse("20060102", string(filename[0:8]))
		if e != nil {
			fmt.Println("日志名称不正确", e)
			continue
		}
		if time.Now().Sub(t.Add(config.LogDaysToKeep*24*time.Hour)) > 0 {
			os.Remove(filepath.Join(config.LogSavePath, filename))
		}
	}
	return nil
}