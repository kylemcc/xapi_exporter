package main

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	xenAPI "github.com/johnprather/go-xen-api-client"
	"github.com/prometheus/client_golang/prometheus"
)

type Entry struct {
	MetricType string // e.g.: AVERAGE
	EntityType string // vm, host
	UUID       string
	Name       string
}

type RrdMetric struct {
	Name   string
	Labels map[string]string
	Value  float64
}

type Legend struct {
	Entries []Entry `xml:"entry"`
}

type Row struct {
	Timestamp int64     `xml:"t"`
	Values    []float64 `xml:"v"`
}

type Data struct {
	Rows []Row `xml:"row"`
}

type RrdMeta struct {
	Start   int64  `xml:"start"`
	Columns int64  `xml:"columns"`
	Legend  Legend `xml:"legend"`
}

type RrdUpdates struct {
	Meta RrdMeta `xml:"meta"`
	Data Data    `xml:"data"`
}

func (l *Entry) UnmarshalXML(d *xml.Decoder, start xml.StartElement) (err error) {
	var e string
	d.DecodeElement(&e, &start)
	*l, err = parseLegendEntry(e)
	return err
}

func parseLegendEntry(s string) (Entry, error) {
	fields := strings.Split(s, ":")
	if len(fields) != 4 {
		return Entry{}, fmt.Errorf("Could not parse Entry from %v", s)
	}

	return Entry{fields[0], fields[1], fields[2], fields[3]}, nil
}

var cpuUtilRe = regexp.MustCompile(`cpu(?P<cpu_num>\d+)(?:-C(?P<core_num>\d+))?`)

func parseCpuMetric(metric string) (string, map[string]string) {
	res := cpuUtilRe.FindStringSubmatch(metric)
	labels := map[string]string{}
	for i, v := range res[1:] {
		labels[cpuUtilRe.SubexpNames()[i+1]] = v
	}
	return "cpu", labels
}

func mapRrds(rrdUpdates []*RrdUpdates,
	hostRecs map[xenAPI.HostRef]xenAPI.HostRecord,
	vmRecs map[xenAPI.VMRef]xenAPI.VMRecord) []*RrdMetric {

	uuidToOpaqueReference := map[string]string{}

	for opaqueReference, hostRecord := range hostRecs {
		uuidToOpaqueReference[hostRecord.UUID] = string(opaqueReference)
	}

	for opaqueReference, vmRecord := range vmRecs {
		uuidToOpaqueReference[vmRecord.UUID] = string(opaqueReference)
	}
	var dataLen int
	for i := 0; i < len(rrdUpdates); i++ {
		dataLen += len(rrdUpdates[i].Data.Rows)
	}

	mapped := make([]*RrdMetric, 0, dataLen)
	var (
		hostname        string
		residentHost    string
		opaqueReference string
		vmrec           xenAPI.VMRecord
		hostrec         xenAPI.HostRecord
	)
	for _, u := range rrdUpdates {
		for i, entry := range u.Meta.Legend.Entries {
			opaqueReference = uuidToOpaqueReference[entry.UUID]

			switch entry.EntityType {
			case "vm":
				vmrec = vmRecs[xenAPI.VMRef(opaqueReference)]
				hostrec = hostRecs[vmrec.ResidentOn]
				hostname = vmrec.NameLabel
				residentHost = hostrec.Hostname
			case "host":
				hostrec = hostRecs[xenAPI.HostRef(opaqueReference)]
				hostname = hostrec.Hostname
				residentHost = ""
			}

			m := RrdMetric{
				Name: entry.Name,
				Labels: map[string]string{
					"uuid":          entry.UUID,
					"hostname":      hostname,
					"resident_host": residentHost,
					"scope":         entry.EntityType,
				},
				Value: u.Data.Rows[0].Values[i],
			}

			if cpuUtilRe.MatchString(m.Name) {
				name, labels := parseCpuMetric(m.Name)
				m.Name = name
				for k, v := range labels {
					m.Labels[k] = v
				}
			}

			mapped = append(mapped, &m)
		}
	}

	return mapped
}

func gatherRRDs(hostRecs map[xenAPI.HostRef]xenAPI.HostRecord) []*RrdUpdates {
	qs := url.Values{}
	tenSecondsAgo := time.Now().Unix() - 10
	qs.Set("start", strconv.Itoa(int(tenSecondsAgo)))
	qs.Set("host", "true")

	u := url.URL{}
	u.Scheme = "https"
	u.Path = "rrd_updates"
	u.RawQuery = qs.Encode()

	resultCh := make(chan *RrdUpdates)
	var updates []*RrdUpdates

	for _, v := range hostRecs {
		u.Host = v.Address
		log.Printf("Requesting RRDs from ID:[%v] hostname:[%v] ip:[%v]\n", v.UUID, v.Hostname, v.Address)
		go func(rrdUrl url.URL) {
			up, err := requestRRD(rrdUrl, resultCh)
			if err != nil {
				log.Printf("error requesting RRD for host [%v]: %v", v.Hostname, err)
			}
			resultCh <- up
		}(u)
	}

	for i := 0; i < len(hostRecs); i++ {
		if up := <-resultCh; up != nil {
			updates = append(updates, up)
		}
	}

	return updates
}

func requestRRD(u url.URL, resultCh chan<- *RrdUpdates) (*RrdUpdates, error) {
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(config.Auth.Username, config.Auth.Password)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	var ru RrdUpdates
	if err := xml.Unmarshal(body, &ru); err != nil {
		return nil, err
	}

	return &ru, nil
}

func appendRrdsMetrics(metricList []*prometheus.GaugeVec, hostRecs map[xenAPI.HostRef]xenAPI.HostRecord, vmRecs map[xenAPI.VMRef]xenAPI.VMRecord) []*prometheus.GaugeVec {
	rrdMetrics := gatherRRDs(hostRecs)
	mappedRecords := mapRrds(rrdMetrics, hostRecs, vmRecs)

	for _, record := range mappedRecords {
		xapiMetric := newMetric(strings.Replace(record.Name, "-", "_", -1), record.Labels, record.Value)
		metricList = append(metricList, xapiMetric)
	}

	return metricList
}
