package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// These endpoints receive a city/coordinates only, never model credentials,
// attachments, conversation history or the original prompt.
const weatherEndpoint = "https://api.open-meteo.com/v1/forecast"
const geocodingEndpoint = "https://geocoding-api.open-meteo.com/v1/search"

func needsWeather(prompt string) bool {
	for _, word := range []string{"天气", "气温", "会下雨", "会不会下雨", "weather"} {
		if strings.Contains(strings.ToLower(prompt), word) {
			return true
		}
	}
	return false
}

func weatherLocation(ctx context.Context, m Model, prompt string) (string, error) {
	// Resolve common explicit names without spending a second model request.
	for _, pair := range [][2]string{{"上海", "Shanghai"}, {"北京", "Beijing"}, {"广州", "Guangzhou"}, {"深圳", "Shenzhen"}, {"杭州", "Hangzhou"}, {"南京", "Nanjing"}, {"成都", "Chengdu"}, {"武汉", "Wuhan"}, {"重庆", "Chongqing"}, {"天津", "Tianjin"}} {
		if strings.Contains(prompt, pair[0]) || strings.Contains(strings.ToLower(prompt), strings.ToLower(pair[1])) {
			return pair[1], nil
		}
	}
	text, err := callModelText(ctx, m, `只提取用户要查询天气的一个城市。返回 JSON {"city":"城市英文名"}，没有明确城市则 city 为空。不要猜测用户所在地，不要返回 URL 或其他内容。`, prompt, nil, true)
	if err != nil {
		return "", err
	}
	var result struct {
		City string `json:"city"`
	}
	if json.Unmarshal([]byte(unfenceText(text)), &result) != nil || len(result.City) > 100 || strings.TrimSpace(result.City) == "" {
		return "", errors.New("请提供要查询天气的城市，例如：上海今天天气")
	}
	return strings.TrimSpace(result.City), nil
}

func weatherJSON(ctx context.Context, client *http.Client, endpoint string, params url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "OpsBuddy/0.8.1")
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("天气服务连接失败或超时")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("天气服务返回 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (256<<10)+1))
	if err != nil || len(body) > 256<<10 {
		return errors.New("天气服务响应不完整或过大")
	}
	if json.Unmarshal(body, out) != nil {
		return errors.New("天气服务未返回有效数据")
	}
	return nil
}

func fetchWeather(ctx context.Context, city string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("天气服务重定向被拒绝") }}
	return fetchWeatherFrom(ctx, client, geocodingEndpoint, weatherEndpoint, city)
}

func fetchWeatherFrom(ctx context.Context, client *http.Client, geo, forecast, city string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var location struct {
		Results []struct {
			Name      string  `json:"name"`
			Country   string  `json:"country"`
			Admin     string  `json:"admin1"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := weatherJSON(ctx, client, geo, url.Values{"name": {city}, "count": {"1"}, "language": {"zh"}, "format": {"json"}}, &location); err != nil {
		return "", err
	}
	if len(location.Results) == 0 {
		return "", errors.New("未找到该城市，请补充城市和国家")
	}
	loc := location.Results[0]
	if loc.Latitude < -90 || loc.Latitude > 90 || loc.Longitude < -180 || loc.Longitude > 180 {
		return "", errors.New("城市坐标无效")
	}
	params := url.Values{"latitude": {fmt.Sprint(loc.Latitude)}, "longitude": {fmt.Sprint(loc.Longitude)}, "current": {"temperature_2m,relative_humidity_2m,apparent_temperature,weather_code,wind_speed_10m"}, "daily": {"weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max"}, "forecast_days": {"7"}, "timezone": {"auto"}, "timeformat": {"unixtime"}}
	var data struct {
		Timezone string `json:"timezone"`
		Offset   int    `json:"utc_offset_seconds"`
		Current  struct {
			Time     int64    `json:"time"`
			Temp     *float64 `json:"temperature_2m"`
			Feels    *float64 `json:"apparent_temperature"`
			Humidity *float64 `json:"relative_humidity_2m"`
			Wind     *float64 `json:"wind_speed_10m"`
			Code     *int     `json:"weather_code"`
		} `json:"current"`
		Daily struct {
			Time []int64    `json:"time"`
			Code []*int     `json:"weather_code"`
			High []*float64 `json:"temperature_2m_max"`
			Low  []*float64 `json:"temperature_2m_min"`
			Rain []*float64 `json:"precipitation_probability_max"`
		} `json:"daily"`
	}
	if err := weatherJSON(ctx, client, forecast, params, &data); err != nil {
		return "", err
	}
	c := data.Current
	if c.Time == 0 || c.Temp == nil || c.Code == nil || time.Since(time.Unix(c.Time, 0)) > 3*time.Hour || time.Until(time.Unix(c.Time, 0)) > time.Hour {
		return "", errors.New("天气数据缺失或已过期，不能作为当前天气")
	}
	zone := time.FixedZone(data.Timezone, data.Offset)
	scalar := func(v *float64, unit string) string {
		if v == nil {
			return "未提供"
		}
		return fmt.Sprintf("%.1f%s", *v, unit)
	}
	result := fmt.Sprintf("城市：%s，%s %s\n数据时间：%s（%s）\n当前：%s，气温 %s，体感 %s，湿度 %s，风速 %s\n", loc.Name, loc.Admin, loc.Country, time.Unix(c.Time, 0).In(zone).Format("2006-01-02 15:04"), data.Timezone, weatherDescription(*c.Code), scalar(c.Temp, "°C"), scalar(c.Feels, "°C"), scalar(c.Humidity, "%"), scalar(c.Wind, " km/h"))
	n := len(data.Daily.Time)
	if n > 7 {
		n = 7
	}
	for i := 0; i < n; i++ {
		if i >= len(data.Daily.High) || i >= len(data.Daily.Low) || i >= len(data.Daily.Code) || i >= len(data.Daily.Rain) {
			return "", errors.New("天气预报数据不完整")
		}
		description := "未提供"
		if data.Daily.Code[i] != nil {
			description = weatherDescription(*data.Daily.Code[i])
		}
		result += fmt.Sprintf("%s：%s，%s ~ %s，降水概率 %s\n", time.Unix(data.Daily.Time[i], 0).In(zone).Format("2006-01-02"), description, scalar(data.Daily.Low[i], "°C"), scalar(data.Daily.High[i], "°C"), scalar(data.Daily.Rain[i], "%"))
	}
	result += "来源：[Open-Meteo](https://open-meteo.com/)；当前数据为天气模型估算，预报可能变化。\n查询链接：" + forecast + "?" + params.Encode() + "\n获取时间：" + time.Now().In(zone).Format(time.RFC3339)
	return result, nil
}

func weatherDescription(code int) string {
	switch code {
	case 0:
		return "晴"
	case 1:
		return "大部晴朗"
	case 2:
		return "多云"
	case 3:
		return "阴"
	case 45, 48:
		return "雾"
	case 51, 53, 55:
		return "毛毛雨"
	case 56, 57, 66, 67:
		return "冻雨"
	case 61:
		return "小雨"
	case 63:
		return "中雨"
	case 65:
		return "大雨"
	case 71, 73, 75, 77:
		return "雪"
	case 80, 81, 82:
		return "阵雨"
	case 85, 86:
		return "阵雪"
	case 95, 96, 99:
		return "雷暴"
	default:
		return fmt.Sprintf("天气代码 %d", code)
	}
}
