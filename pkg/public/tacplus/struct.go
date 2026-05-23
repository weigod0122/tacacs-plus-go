package tacplus

import "time"

type AccountInfo struct {
	StartTime       time.Time `json:"-"`
	Time            string    `json:"time"`
	TimeStamp       int64     `json:"timeStamp"`
	TimeRange       int64     `json:"timeRange"`
	User            string    `json:"user"`
	SwitchAddr      string    `json:"switchAddr"`
	ServerAddr      string    `json:"serverAddr"`
	Cmd             string    `json:"cmd"`
	Port            string    `json:"port"`
	Flags           int16     `json:"flags"`
	AuthenMethod    int16     `json:"authenMethod"`
	PrivLvl         int16     `json:"privLvl"`
	AuthenType      int16     `json:"authenType"`
	AuthenService   int16     `json:"authenService"`
	Arg             []string  `json:"arg"`
	IsSingleConnect bool      `json:"isSingleConnect"`
	TacacsClient    string    `json:"tacacsClient"`
}

type AuthorInfo struct {
	StartTime       time.Time `json:"-"`
	Time            string    `json:"time"`
	TimeStamp       int64     `json:"timeStamp"`
	TimeRange       int64     `json:"timeRange"`
	User            string    `json:"user"`
	SwitchAddr      string    `json:"switchAddr"`
	ServerAddr      string    `json:"serverAddr"`
	AuthorStatus    string    `json:"authorStatus"`
	Details         string    `json:"details"`
	Cmd             string    `json:"cmd"`
	IsSingleConnect bool      `json:"isSingleConnect"`
	TacacsClient    string    `json:"tacacsClient"`
}

type AuthenInfo struct {
	StartTime       time.Time `json:"-"`
	Time            string    `json:"time"`
	TimeStamp       int64     `json:"timeStamp"`
	TimeRange       int64     `json:"timeRange"`
	User            string    `json:"user"`
	SwitchAddr      string    `json:"switchAddr"`
	ServerAddr      string    `json:"serverAddr"`
	AuthenStatus    string    `json:"authenStatus"`
	Details         string    `json:"details"`
	IsSingleConnect bool      `json:"isSingleConnect"`
	TacacsClient    string    `json:"tacacsClient"`
}
