package service

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/Sirupsen/logrus"
	simplejson "github.com/bitly/go-simplejson"
	cache "github.com/patrickmn/go-cache"
	"github.com/rancher/rancher-auth-filter-service/manager"
)

//RequestData is for the JSON output
type RequestData struct {
	Headers map[string][]string    `json:"headers,omitempty"`
	Body    map[string]interface{} `json:"body,omitempty"`
}

//ValidationHandler is a handler for cookie token and returns the request headers and accountid and projectid
func ValidationHandler(w http.ResponseWriter, r *http.Request) {

	reqestData := RequestData{}
	input, err := ioutil.ReadAll(r.Body)
	jsonInput, _ := simplejson.NewJson(input)
	json.Unmarshal(input, &reqestData)
	cookieString, err := jsonInput.Get("headers").Get("Cookie").GetIndex(0).String()
	tokens := strings.Split(cookieString, ";")
	var tokenValue string
	for i := range tokens {
		if strings.Contains(tokens[i], "token") {
			tokenValue = strings.Split(tokens[i], "=")[1]
		}

	}

	if err == nil {
		//check if the token value is empty or not
		if tokenValue != "" {
			var projectID []string
			logrus.Infof("token:" + tokenValue)
			cachedValue, foundInCache := manager.CacheProjectID.Get(tokenValue)
			//if token find in the memory cache, then will not call the rancher api
			if foundInCache {
				cacheprojectid := cachedValue.([]string)
				projectID = cacheprojectid
			} else {
				projectID = getValue(manager.URL, "projects", tokenValue)
			}

			//check if the accountID or projectID is empty
			if projectID[0] != "" {
				if projectID[0] == "Unauthorized" {
					w.WriteHeader(401)
					logrus.Infof("Token " + tokenValue + " is not valid.")
				} else if projectID[0] == "ID_NOT_FIND" {
					w.WriteHeader(501)
					logrus.Infof("Cannot provide the service. Please check the rancher server URL." + manager.URL)
				} else {
					//construct the responseBody
					var headerBody = make(map[string][]string)
					var Body = make(map[string]interface{})

					requestHeader := reqestData.Headers
					for k, v := range requestHeader {
						headerBody[k] = v
					}
					requestBody := reqestData.Body
					for k, v := range requestBody {
						Body[k] = v
					}
					//if the token not find in cache, add new token into cache
					if !foundInCache {
						manager.CacheProjectID.Set(tokenValue, projectID, cache.DefaultExpiration)
						logrus.Infof("Token " + tokenValue + " set into the cache.")
					}

					// _, found := manager.CacheProjectID.Get(tokenValue)
					// fmt.Println("Find the cache value after set into cache: " + strconv.FormatBool(found))
					headerBody["X-API-Project-Id"] = projectID

					var responseBody RequestData
					responseBody.Headers = headerBody
					responseBody.Body = Body
					//convert the map to JSON format
					if responseBodyString, err := json.Marshal(responseBody); err != nil {
						panic(err)
					} else {
						w.WriteHeader(http.StatusOK)
						w.Write(responseBodyString)
					}
				}
			}

		} else {
			logrus.Infof("No token found")
			w.WriteHeader(401)
		}

	} else {
		logrus.Infof("No token found")
		w.WriteHeader(401)
	}
}

//get the projectID and accountID from rancher API
func getValue(host string, path string, token string) []string {
	var result []string
	client := &http.Client{}
	requestURL := host + "v2-beta/" + path
	req, err := http.NewRequest("GET", requestURL, nil)
	cookie := http.Cookie{Name: "token", Value: token}
	req.AddCookie(&cookie)
	resp, err := client.Do(req)
	if err != nil {
		logrus.Fatal(err)
	}
	bodyText, err := ioutil.ReadAll(resp.Body)
	js, _ := simplejson.NewJson(bodyText)
	authorized, _ := js.Get("message").String()

	if authorized == "Unauthorized" {
		result = []string{"Unauthorized"}
	} else {
		var id string
		jsonBody, _ := simplejson.NewJson(bodyText)
		dataLenth := len(jsonBody.Get("data").MustArray())
		for i := 0; i < dataLenth; i++ {
			id, err = jsonBody.Get("data").GetIndex(i).Get("id").String()

			if err != nil {
				logrus.Info(err)
				result = []string{"ID_NOT_FIND"}
			} else {
				result = append(result, id)
			}
		}

	}

	return result
}
