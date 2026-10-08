package httpapi

import (
	"github.com/gin-gonic/gin"
	"nicetools/backend/internal/service"
)

func (a *API) users(c *gin.Context) {
	rows, err := a.S.Users()
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, rows)
}
func (a *API) createUser(c *gin.Context) {
	var input service.UserInput
	if !bind(c, &input) {
		return
	}
	row, err := a.S.CreateUser(current(c).ID, input)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(201, row)
}
func (a *API) updateUser(c *gin.Context) {
	id, ok := numericID(c)
	if !ok {
		return
	}
	var input service.UserInput
	if !bind(c, &input) {
		return
	}
	row, err := a.S.UpdateUser(current(c).ID, id, input)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, row)
}
func (a *API) deleteUser(c *gin.Context) {
	id, ok := numericID(c)
	if !ok {
		return
	}
	if err := a.S.DeleteUser(current(c).ID, id); err != nil {
		a.fail(c, err)
		return
	}
	c.Status(204)
}
func (a *API) groups(c *gin.Context) {
	rows, err := a.S.Groups()
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, rows)
}
func (a *API) createGroup(c *gin.Context) {
	var input service.GroupInput
	if !bind(c, &input) {
		return
	}
	row, err := a.S.SaveGroup(current(c).ID, 0, input)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(201, row)
}
func (a *API) updateGroup(c *gin.Context) {
	id, ok := numericID(c)
	if !ok {
		return
	}
	var input service.GroupInput
	if !bind(c, &input) {
		return
	}
	row, err := a.S.SaveGroup(current(c).ID, id, input)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, row)
}
func (a *API) deleteGroup(c *gin.Context) {
	id, ok := numericID(c)
	if !ok {
		return
	}
	if err := a.S.DeleteGroup(current(c).ID, id); err != nil {
		a.fail(c, err)
		return
	}
	c.Status(204)
}
func (a *API) policies(c *gin.Context) {
	rows, err := a.S.Policies()
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, rows)
}
func (a *API) savePolicy(c *gin.Context) {
	var input service.PolicyInput
	if !bind(c, &input) {
		return
	}
	row, err := a.S.SavePolicy(current(c).ID, c.Param("id"), input)
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, row)
}
func (a *API) audit(c *gin.Context) {
	rows, err := a.S.Audits()
	if err != nil {
		a.fail(c, err)
		return
	}
	c.JSON(200, rows)
}
func (a *API) password(c *gin.Context) {
	var input struct {
		Current  string `json:"current_password"`
		Password string `json:"password"`
	}
	if !bind(c, &input) {
		return
	}
	if err := a.S.ChangePassword(current(c).ID, input.Current, input.Password); err != nil {
		a.fail(c, err)
		return
	}
	a.clearCookies(c)
	c.Status(204)
}
