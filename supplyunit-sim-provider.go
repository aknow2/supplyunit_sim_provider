package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/planar"

	pGeo "github.com/synerex/proto_geography"
	pb "github.com/synerex/synerex_api"

	pbase "github.com/synerex/synerex_proto"
	sxutil "github.com/synerex/synerex_sxutil"
)

var (
	startPoints = "startPoints.geojson"
	obstacleMap = "2-wall.geojson"
	RAND_MAX    = 32767
)

var (
	nodesrv         = flag.String("nodesrv", "127.0.0.1:9990", "Node ID Server")
	port            = flag.Int("port", 1070, "People Counter Provider Listening Port")
	num             = flag.Int("num", 11, "Number of agents")
	mu              sync.Mutex
	version         = "0.01"
	sxServerAddress string
)

// just for stat debug
func monitorStatus() {
	for {
		sxutil.SetNodeStatus(int32(0), "PC-Sim")
		time.Sleep(time.Second * 3)
	}
}

func loadGeoJson(fname string) *geojson.FeatureCollection {
	bytes, err := ioutil.ReadFile(fname)
	if err != nil {
		log.Print("Can't read file:", err)
		panic("load json")
	}

	fc, _ := geojson.UnmarshalFeatureCollection(bytes)

	return fc
}

func setStartPoygons(fcs *geojson.FeatureCollection) []*orb.Polygon {
	fclen := len(fcs.Features)
	polygons := make([]*orb.Polygon, 0, fclen)
	log.Printf("Fetures: %d", fclen)
	for i := 0; i < fclen; i++ {
		geom := fcs.Features[i].Geometry
		log.Printf("MultiPolygon %d: %v", i, geom)
		if geom.GeoJSONType() == "MultiPolygon" {
			mp := geom.(orb.MultiPolygon)
			ll := len(mp)
			for j := 0; j < ll; j++ {
				poly := mp[j].Clone()
				polygons = append(polygons, &poly)
			}
		}
	}
	return polygons
}

func getStartPoint(pgn *orb.Polygon) orb.Point {
	// random.
	bd := pgn.Bound()
	dx := bd.Max[0] - bd.Min[0]
	dy := bd.Max[1] - bd.Min[1]
	ct := bd.Center()

	count := 0

	for {
		ddx := dx * rand.Float64()
		ddy := dy * rand.Float64()
		pt := orb.Point{ct[0] - dx/2 + ddx, ct[1] - dy/2 + ddy}
		if planar.PolygonContains(*pgn, pt) {
			return pt
		}
		count++
		//		if count %10 == 0 {
		log.Printf("Check StartPoint:%d", count) // in case of ...
		//		}
	}

}

type Pos struct {
	Lat   float64
	Lon   float64
	Label string
}

var posList = []*Pos{
	&Pos{
		Lon:   136.881161,
		Lat:   35.168587,
		Label: "名古屋駅",
	},
	&Pos{
		Lon:   136.898981,
		Lat:   35.187595,
		Label: "名古屋城",
	},
	&Pos{
		Lon:   136.970909,
		Lat:   35.154811,
		Label: "名古屋大学",
	},
	&Pos{
		Lon:   136.811031,
		Lat:   35.374950,
		Label: "アクアトト・ぎふ",
	},
	&Pos{
		Lon:   136.730681,
		Lat:   35.035158,
		Label: "長島スパーランド",
	},
	&Pos{
		Lon:   136.843527,
		Lat:   35.050638,
		Label: "レゴランド",
	},
	&Pos{
		Lon:   136.811778,
		Lat:   34.863682,
		Label: "セントレア",
	},
}

func updateVisualization(clt *sxutil.SXServiceClient) {
	//	fmt.Printf("Time: %v\n", sim.GetGlobalTime())
	bars := make([]*pGeo.BarGraph, 0, *num)

	for i, pos := range posList {
		bars = append(bars, &pGeo.BarGraph{
			Id: int32(i),
			Ts: &timestamp.Timestamp{
				Seconds: time.Now().Unix(),
			},
			Type:   pGeo.BarType_BT_BOX_VARCOLOR,
			Color:  rand.Int31n(0xFFFF),
			Lon:    pos.Lon,
			Lat:    pos.Lat,
			Width:  300,
			Radius: 900,
			BarData: []*pGeo.BarData{
				&pGeo.BarData{
					Value: rand.Float64() * 300,
					Label: "食料",
					Color: 0xff0000,
				},
				&pGeo.BarData{
					Value: rand.Float64() * 300,
					Label: "水",
					Color: 0x00FF00,
				},
				&pGeo.BarData{
					Value: rand.Float64() * 300,
					Label: "毛布",
					Color: 0x0000FF,
				},
			},
			Min:  100,
			Max:  1500,
			Text: pos.Label,
		})
	}
	barList := pGeo.BarGraphs{
		Bars: bars,
	}
	out, _ := proto.Marshal(&barList)
	cont := pb.Content{Entity: out}
	smo := sxutil.SupplyOpts{
		Name:  "BarGraphs",
		Cdata: &cont,
	}
	_, nerr := clt.NotifySupply(&smo)
	if nerr != nil { // connection failuer with current client
		log.Printf("Connection failure", nerr)
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())
	flag.Parse()
	go sxutil.HandleSigInt()
	wg := sync.WaitGroup{} // for syncing other goroutines

	//	ln := len(pgs)
	name := "SU-Sim"
	sxutil.RegisterDeferFunction(sxutil.UnRegisterNode)
	channelTypes := []uint32{pbase.PEOPLE_COUNTER_SVC, pbase.PEOPLE_AGENT_SVC}
	srv, rerr := sxutil.RegisterNode(*nodesrv, name, channelTypes, nil)
	if rerr != nil {
		log.Fatal("Can't register node ", rerr)
	}
	log.Printf("Connecting SynerexServer at [%s]\n", srv)

	client := sxutil.GrpcConnectServer(srv)
	sxServerAddress = srv
	argJSON := fmt.Sprintf(name)

	supplyClient := sxutil.NewSXServiceClient(client, pbase.GEOGRAPHIC_SVC, argJSON)
	wg.Add(1)
	go monitorStatus() // keep status
	log.Printf("Starting Supplyunit Simulator")

	count := 0
	for {
		if count%10 == 0 {
			log.Printf("Update Supplyunit")
			updateVisualization(supplyClient)
			time.Sleep(5000 * time.Millisecond)
		}
		count++
	}
	wg.Wait()
}
