package controllers

import (
	"github.com/astaxie/beego"
)

type MainController struct {
	beego.Controller
}

func (this *MainController) Get() {
	//set templater
	this.Data["PageTitle"] = "Nemo's blog"
	this.Data["IsHome"] = true
	this.TplNames = "home.html"

}