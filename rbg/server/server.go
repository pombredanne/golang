package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"gotest/rbg/config"
	"gotest/rbg/logs"
	"gotest/rbg/task"
	"io/ioutil"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	server_preferences *config.ServerConfig    // 配置文件实例
	dao                *sql.DB                 // 数据库实例
	log                *logs.BeeLogger         // 日志实例
	clientConns        map[string]*net.TCPConn // 每个客户端的连接集合
	smtp               *sql.Stmt
)

// 需要传输数据的结构
type Obj struct {
	Date                string    // 日期
	Time                string    // 时间
	InTime              time.Time // 插入时间
	SerialNumber        string    // 流水号
	Type                string    // 交易类型
	CardId              string    // 预留卡号
	FaceValue           int       // 面值
	Version             int       // 版本号
	CurrencyCode        int       // 币种
	SerialNumberInTimes int       // 该钞在本笔交易内序号
	CurrencyNumber      string    // 冠字号码
	Ima                 []byte    // 冠字号图像数据
	ImaPath             string    // 冠字号保存图像路径
	ClientName          string    // 客户端设备名称
	ClientIP            string    // 客户端IP
	Remark              string    // 备注
}

// 接收数据处理方法
func (o *Obj) SendToServer(obj *Obj, replay *string) error {
	// 图像保存
	if "" != obj.ImaPath {
		obj.ImaPath = filepath.Join(server_preferences.BMP_SAVE_PATH, obj.ClientName, obj.Date, obj.SerialNumber+".bmp")
	REGO:
		f, err := os.Create(obj.ImaPath)
		if err != nil {
			err = os.MkdirAll(obj.ImaPath[:strings.LastIndex(obj.ImaPath, "\\")], 0666)
			if err != nil {
				log.Error("保存bmp失败：%s", obj.SerialNumber)
				*replay = config.SAVE_BMP_ERROR
				return nil
			}
			log.Info("创建目录：", obj.ClientName)
			goto REGO
		}
		defer f.Close()
		f.Write(obj.Ima)
	}

	// 数据存库

	//insert_sql := "INSERT INTO T_BR(SDATE,STIME,INTIME,CARDID,BILLNO,BILLBN) VALUES(?,?,?,?,?,?)"
	str_time, _ := time.Parse("2006-01-02 15:04:05", (obj.Date[0:4] + "-" + obj.Date[4:6] + "-" + obj.Date[6:8] + " " + obj.Time))
	_, err := smtp.Exec(obj.Date, obj.Time, str_time, obj.SerialNumber, obj.Type, obj.CardId, obj.FaceValue, obj.Version, obj.CurrencyCode, obj.SerialNumberInTimes, obj.CurrencyNumber, obj.ImaPath, obj.ClientName, obj.ClientIP, obj.Remark)
	if err != nil {
		log.Error("%s%s", "保存到数据库失败：", obj.CurrencyNumber)
		log.Error("%s", err)
		*replay = config.SAVE_TO_DB_ERROR
		return nil
	}
	*replay = "OK"
	return nil
}

func init() {
	loadConfig()
	// 日志初始化
	log = logs.NewLogger(10000)
	// 日志文件记录
	logfile := filepath.Join(pwd, "logs", "server.log")
	os.MkdirAll(logfile[0:len(logfile)-10], 0666)
	_, e = os.Stat(logfile)
	if nil != e {
		os.Create(logfile)
	}
	log.SetLogger("file", `{"filename":"`+strings.Replace(logfile, "\\", "/", -1)+`"}`)
	// 日志终端记录
	log.SetLogger("console", "")

	openDB()
}

func main() {
	// 以下为清理过期数据，每天22点，23点各执行一次
	dataClearTask := task.NewTask("dataClearTask", "00 00 22,23 * * * ", dataClear)
	task.AddTask("dataClearTask", dataClearTask)
	task.StartTask()

	sql := "INSERT INTO T_BR(DATE,TIME,INTIME,SERIALNUMBER,TYPE,CARDID,FACEVALUE,VERSION,CURRENCYCODE,SERIALNUMBERINTIMES,BILLBN,IMAPATH,CLIENTNAME,CLIENTIP,REMARK)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)"
	var e error
	smtp, e = dao.Prepare(sql)
	if e != nil {
		log.Error(e.Error())
	}
	defer smtp.Close()
	cpus := runtime.NumCPU()
	runtime.GOMAXPROCS(cpus)
	defer dao.Close()
	clientConns = make(map[string]*net.TCPConn, 100)

	u := new(Obj)
	rpc.Register(u)

	go func() {
		for {
			select {
			case <-time.After(3 * time.Second):
				for k, v := range clientConns {
					log.Info("%s,%v", k, v)
				}
			}
		}
	}()

	// http：方式
	exit := make(chan bool)
	rpc.HandleHTTP()
	err := http.ListenAndServe(server_preferences.SERVER_IP_PORT, nil)
	checkError(err)
	<-exit

	// tcp 方式
	// tcpAddr, err := net.ResolveTCPAddr("tcp", server_preferences.SERVER_IP_PORT)
	// checkError(err)
	// listener, err := net.ListenTCP("tcp", tcpAddr)
	// checkError(err)

	// log.Info("服务端已经启动!")
	// for {
	// 	conn, err := listener.AcceptTCP()
	// 	conn.Write([]byte("ok"))
	// 	if err != nil {
	// 		log.Error("rpc.Server: accept Error:%s", err)
	// 	}
	// 	ip := strings.Split(conn.RemoteAddr().String(), ":")[0]

	// 	if v, ok := clientConns[ip]; ok {
	// 		v.Close()
	// 		continue
	// 	}
	// 	clientConns[ip] = conn
	// 	conn.SetKeepAlive(true)
	// 	conn.SetKeepAlivePeriod(120 * time.Second)
	// 	log.Info("IP [ %s ] 已经成功连接到服务器...[%d]", ip, len(clientConns))
	// 	go rpc.ServeConn(conn)
	// }

}

// 加载配置文件
func loadConfig() {
	pwd, _ := os.Getwd()
	file, e := ioutil.ReadFile(filepath.Join(pwd, "Server.Preferences.json"))
	if e != nil {
		fmt.Println("读取配置文件失败!请与管理员联系!" + e.Error())
		os.Exit(1)
	}

	logfile := filepath.Join(pwd, "logs", "server.log")
	os.MkdirAll(logfile[0:len(logfile)-10], 0666)
	log = logs.NewLogger(100000)
	log.SetLogger("file", `{"filename":"`+strings.Replace(logfile, "\\", "/", -1)+`"}`)
	log.SetLogger("console", "")
	server_preferences = new(config.ServerConfig)
	e = json.Unmarshal(file, server_preferences)
	if e != nil {
		os.Exit(2)
	}
}

// 获取数据库
func openDB() {
	db, err := sql.Open("mysql", server_preferences.DATABASE_USER_NAME+":"+server_preferences.DATABASE_PASSWORD+"@tcp(127.0.0.1:3306)/"+server_preferences.DATABASE_NAME+"?charset=utf8") // &timeout=60s
	if err != nil {
		errmsg := "错误：连接数据库连接失败!"
		fmt.Println(errmsg)
		os.Exit(1)
	}
	err = db.Ping()
	if err != nil {
		fmt.Println(">>>>", err.Error())
		os.Exit(1)
	}

	db.SetMaxIdleConns(server_preferences.DB_MAX_IDLE_CONNS)
	db.SetMaxOpenConns(server_preferences.DB_MAX_OPEN_CONNS)
	dao = db

}

// 关闭客户端连接
func CloseConn() {
	if nil != dao {
		dao.Close()
		dao = nil
	}
}

func checkError(err error) {
	if err != nil {
		log.Warn("Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

// 清理过期数据
func dataClear() error {
	dayLastYear := time.Now().Add(time.Hour * -(server_preferences.DATA_KEEPING_DAYS * 24)).Format("20060102")
	_, err := dao.Exec("DELETE FROM T_BR WHERE DATE = ? ", dayLastYear)
	if err != nil {
		log.Warn("删除数据库过期数据失败: %s", err.Error())
		return errors.New("删除数据库过期数据失败：" + err.Error())
	}
	files, err := ioutil.ReadDir(server_preferences.BMP_SAVE_PATH)
	if err != nil {
		log.Warn("未找到BMP目录: %s", err.Error())
		return errors.New("未找到BMP目录：" + err.Error())
	}
	for _, file := range files {
		clientName := file.Name()
		fmt.Println(server_preferences.BMP_SAVE_PATH, clientName, dayLastYear)
		err = os.RemoveAll(filepath.Join(server_preferences.BMP_SAVE_PATH, clientName, dayLastYear))
		if err != nil {
			log.Warn("删除过期BMP失败: %s", err.Error())
			return errors.New("删除过期BMP失败:" + err.Error())
		}
	}
	log.Info("清理过期数据成功！")
	return nil
}
