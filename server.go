package main
import (
	"os/exec"
        "encoding/json"
        "fmt"
	"strings"
	"io/ioutil"
	"os"
        "log"
        "net/http"
        "gopkg.in/mgo.v2"
        "gopkg.in/mgo.v2/bson"
        "github.com/gorilla/mux"
)

type Person struct {
	Id string	`bson:"id"`
        Name    string
	Address string
	City string
	State string
	Zip string
	Coordinate	Coordinate //`bson:"inline"`
}

type Coordinate struct {
	Lat float64
	Lng float64
}	

type GoogleResponse struct {
        Results []*GoogleResult
	Status	     string 		   `json:"status"`
}

type GoogleResult struct {
        Address      string               `json:"formatted_address"`
        AddressParts []*GoogleAddressPart `json:"address_components"`
        Geometry     *Geometry
        Types        []string
}

type GoogleAddressPart struct {
        Name      string `json:"long_name"`
        ShortName string `json:"short_name"`
        Types     []string
}

type Geometry struct {
        Bounds   Bounds
        Location Point
        Type     string
        Viewport Bounds
}

type Bounds struct {
        NorthEast, SouthWest Point
}

type Point struct {
        Lat, Lng float64
}

type Server struct {
  dbsession *mgo.Session
  dbcoll *mgo.Collection
}


func main() {
        router := mux.NewRouter()
        router.HandleFunc("/locations/{id}", handleGetReq).Methods("GET")
        router.HandleFunc("/locations/", handlePostReq).Methods("POST")
	router.HandleFunc("/locations/{id}", handlePutReq).Methods("PUT")
	router.HandleFunc("/locations/{id}", handleDeleteReq).Methods("DELETE")
        http.ListenAndServe(":8080", router)
}

func uniqueIDGen() string {
	out, err := exec.Command("uuidgen").Output()
    	if err != nil {
        	log.Fatal(err)
    	}   
       	s := string(out[:36])
	return s
}

func establishDbConn() *Server{
	session, err := mgo.Dial("mongodb://saro:1234@ds043714.mongolab.com:43714/contactdb")
        if err != nil {
                panic(err)
        }   
        session.SetMode(mgo.Monotonic, true)	
	c := session.DB("contactdb").C("contactCollection")
	return &Server{dbsession:session,dbcoll:c}
}

func (s *Server) Close() {
  s.dbsession.Close()
}

func validateInput(userEntry Person) bool {
	var flag bool = true
	if(userEntry.Name == "" || userEntry.Address == "" || userEntry.City =="" || userEntry.State == "" || userEntry.Zip =="") {
		flag = false
	} 
	return flag
}
 
func handleGetReq(res http.ResponseWriter, req *http.Request) {
	conn := establishDbConn()
	defer conn.dbsession.Close()
	c := conn.dbcoll 

        res.Header().Set("Content-Type", "application/json")
        result := Person{} 
	vars := mux.Vars(req)

        err := c.Find(bson.M{"id": vars["id"]}).One(&result) 
        if err != nil {
		fmt.Fprint(res, "The requested id does not correspond to any entries")
                return
	}   

        outgoingJSON, error := json.Marshal(result)
        if error != nil {
                log.Println(error.Error())
                http.Error(res, error.Error(), http.StatusInternalServerError)
                return
        }   
        	fmt.Fprint(res, string(outgoingJSON))
}

func handlePostReq(res http.ResponseWriter, req *http.Request) {
	var latLngArr [2]float64

	conn := establishDbConn()
        defer conn.dbsession.Close()
        c := conn.dbcoll 

	res.Header().Set("Content-Type", "application/json")
        person := new(Person)
        decoder := json.NewDecoder(req.Body)
        error := decoder.Decode(&person)
	
        if error != nil {
			fmt.Fprint(res, "Input format invalid. Input should be in JSON format only!!")
                        return
                }

	if(!validateInput(*person)) {
		fmt.Fprint(res, "Input missing one or more fields. Input should contain Name, Address, State, City and Zip fields in Json format")
		return
	} 
	person.Id=uniqueIDGen()
	
	latLngArr=getLatLng(person.Address,person.City,person.State,person.Zip);
	if(latLngArr[0] == -1){
		//addr was wrong probably. geocode was successful but returned no results 
		fmt.Fprint(res, "Your address could not be processed. Please check if the values of the address, city, state and zip are correct.")
		return
        }  else if (latLngArr[0] == -2) {
		//indicates that the query (address, components or latlng) is missing
		fmt.Fprint(res, "Your address could not be processed. Please check if the values of the address, city, state and zip are present") 
		return	
	} else if( latLngArr[0] == -3) {
		//request could not be processed due to a server error. Try again later
		fmt.Fprint(res, "Server error. Please try again later!") 
	} else {
		person.Coordinate.Lat=latLngArr[0]
		person.Coordinate.Lng=latLngArr[1]	
		
		err := c.Insert(person)
        	if err != nil {
                	panic(err)
        	}
		}
		
       		outgoingJSON, err1 := json.Marshal(person)
                if err1 != nil {
                        log.Println(err1.Error())
                        http.Error(res, err1.Error(), http.StatusInternalServerError)
                        return
                }   
                res.WriteHeader(http.StatusCreated)
                fmt.Fprint(res, string(outgoingJSON))
}

func getLatLng(addr string, city string, state string, zip string) [2]float64 {
	var temp string
	var url string
	var latLngArr [2]float64
	var statusFromApi string
	temp=addr+ ",+" +city+ ",+" + state + ",+" + zip
	temp=strings.Replace(temp," ","+",-1)
	url = "https://maps.googleapis.com/maps/api/geocode/json?address="+temp+"&key=AIzaSyDSY804D6M0CN4stTzJ1Of6gHss7IoJmyk"
	apiResponse, err := http.Get(url)
        if err != nil {
        fmt.Printf("%s", err)
        os.Exit(1)
        } else {
        defer apiResponse.Body.Close()
        contents, err := ioutil.ReadAll(apiResponse.Body)
        if err != nil {
            fmt.Printf("%s", err)
            os.Exit(1)
        }   
        var temp GoogleResponse
        err1 := json.Unmarshal(contents, &temp)
        if err1 != nil {
              fmt.Printf("%s", err1)
              os.Exit(1)
          }
	statusFromApi = temp.Status
	if(statusFromApi == "OK") {
		latLngArr[0]=temp.Results[0].Geometry.Location.Lat
		latLngArr[1]=temp.Results[0].Geometry.Location.Lng 
	} else if (statusFromApi == "ZERO_RESULTS"){
		latLngArr[0] = -1	
		
	} else if (statusFromApi == "INVALID_REQUEST") {
		latLngArr[0] = -2	
		
	} else if(statusFromApi == "UNKNOWN_ERROR") {
		latLngArr[0] = -3
	} 
	}   
	return latLngArr
}

func handlePutReq(res http.ResponseWriter, req *http.Request) {
	var latLngArr [2]float64
	conn := establishDbConn()
        defer conn.dbsession.Close()
        c := conn.dbcoll 
	
	res.Header().Set("Content-Type", "application/json")
        person := new(Person)
        decoder := json.NewDecoder(req.Body)
        error := decoder.Decode(&person)
        if error != nil {
                        fmt.Fprint(res, "Input format invalid. Input should be in JSON format only!!")
                        return
                }   

	if(person.Address == "" || person.City =="" || person.State == "" || person.Zip =="") {	
		fmt.Fprint(res, "Input missing one or more fields. To update, input should contain Address, State, City and Zip in JSON format")
		return
	}
	vars := mux.Vars(req)
        latLngArr=getLatLng(person.Address,person.City,person.State,person.Zip);
	if(latLngArr[0] == -1){
               	//addr was wrong probably. geocode was successful but returned no results 
                fmt.Fprint(res, "Your address could not be processed. Please check if the values of the address, city, state and zip are correct.")
                return
        }  else if (latLngArr[0] == -2) {
                //indicates that the query (address, components or latlng) is missing
                fmt.Fprint(res, "Your address could not be processed. Please check if the values of the address, city, state and zip are present") 
                return  
        } else if( latLngArr[0] == -3) {
                //request could not be processed due to a server error. Try again later
                fmt.Fprint(res, "Server error. Please try again later!") 
        } else {
		person.Coordinate.Lat=latLngArr[0]
                person.Coordinate.Lng=latLngArr[1]
	
		colQuerier := bson.M{"id": vars["id"]}
        	change := bson.M{"$set": bson.M{"address": person.Address, "city":person.City, "state": person.State,"zip":person.Zip, "coordinate.lat":latLngArr[0], "coordinate.lng":latLngArr[1] }}
        	UpdateErr := c.Update(colQuerier, change)
        	if UpdateErr != nil {
			fmt.Fprint(res, "The requested id does not correspond to any entries")
			return
		}
	}
	person.Id=vars["id"]
	outgoingJSON, err := json.Marshal(person)
           	if err != nil {
                        log.Println(error.Error())
                        http.Error(res, err.Error(), http.StatusInternalServerError)
                        return
                }   
	res.WriteHeader(http.StatusCreated)
        fmt.Fprint(res, string(outgoingJSON))
} 


func handleDeleteReq(res http.ResponseWriter, req *http.Request) {
	conn := establishDbConn()
        defer conn.dbsession.Close()
        c := conn.dbcoll 
        vars := mux.Vars(req)
	err := c.Remove( bson.M{"id": vars["id"]})
	if err != nil {
		fmt.Fprint(res, "The requested id does not correspond to any entries")
		return
	}
}
