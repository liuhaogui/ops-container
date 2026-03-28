package model

type HealthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

type VersionResponse struct {
	Version   string `json:"Version"`
	BuildTime string `json:"BuildTime"`
	GoVersion string `json:"GoVersion"`
	GitHash   string `json:"GitHash"`
}

type ResponseBody struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

type StringDataResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data string `json:"data"`
}

type StringListResponse struct {
	Code int      `json:"code"`
	Msg  string   `json:"msg"`
	Data []string `json:"data"`
}

type VersionDataResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data VersionResponse `json:"data"`
}

type User struct {
	ID    uint   `json:"id" gorm:"primaryKey"`
	Name  string `json:"name"`
	Email string `json:"email" gorm:"uniqueIndex"`
}
