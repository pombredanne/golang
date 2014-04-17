package main

import (
	"encoding/csv"
	"os"
)

func main() {
	f, err := os.Create("haha2.xls")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	f.WriteString("\xEF\xBB\xBF") // 写入UTF-8 BOM

	w := csv.NewWriter(f)
	csv.
		w.Write([]string{"编号", "姓名", "年龄"})
	w.Write([]string{"1", "张三", "23"})
	w.Write([]string{"2", "李四", "24"})
	w.Write([]string{"3", "王五", "25"})
	w.Write([]string{"4", "赵六", "26"})
	w.Flush()
}
