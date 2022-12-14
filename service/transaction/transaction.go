package transaction

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"reporting/repository/transaction"
	"reporting/schema"
	"time"

	"github.com/jinzhu/now"
)

type TransactionService interface {
	Report(ctx context.Context, req TrxRequest) (*TrxResponse, error)
	Reporting(ctx context.Context, req *TrxRequest) (*TrxResponse, error)
}

type Transaction struct {
	TransactionRepo transaction.TransactionRepository
}

type TrxRequest struct {
	MerchantID   uint64
	MerchantName string
	OutletID     uint64
	OutletName   string
	Date         string
	StartDate    time.Time
	EndDate      time.Time
	Limit        int
	Page         int
}

func (f *TrxRequest) IsValidRangeDate() bool {
	if f.StartDate.IsZero() || f.EndDate.IsZero() {
		return false
	}

	if f.StartDate.After(f.EndDate) {
		return false
	}

	return true
}

type Pagination struct {
	Limit     int `json:"limit"`
	Page      int `json:"page"`
	TotalPage int `json:"total_page"`
}
type Link struct {
	Current  string `json:"current"`
	NextPage string `json:"next"`
	PervPage string `json:"prev"`
}

type TrxResponse struct {
	Pagination Pagination                 `json:"pagination"`
	Link       Link                       `json:"link"`
	Data       []schema.TransactionReport `json:"data"`
}

func (t *Transaction) Report(ctx context.Context, req TrxRequest) (*TrxResponse, error) {
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Page <= 0 {
		req.Page = 1
	}

	tDate, _ := time.Parse("2006-01", req.Date)
	if tDate.IsZero() {
		tDate = time.Now()
	}

	res, count, err := t.TransactionRepo.Report(ctx, transaction.ReportFilter{
		MerchantID: req.MerchantID,
		OutletID:   req.OutletID,
		Date:       tDate,
		Limit:      req.Limit,
		Page:       req.Page,
	})
	if err != nil {
		return nil, err
	}

	totalPage := math.Floor(float64(count) / float64(req.Limit))
	if totalPage <= 0 {
		totalPage = 1
	}

	uri := "http://localhost:3000/report"
	URL, _ := url.Parse(uri)
	q := URL.Query()
	if req.OutletID > 0 {
		q.Add("outlet_id", fmt.Sprint(req.OutletID))
	}
	q.Add("limit", fmt.Sprint(req.Limit))
	q.Add("page", fmt.Sprint(req.Page))
	URL.RawQuery = q.Encode()

	link := Link{
		Current: URL.String(),
	}

	if totalPage > 1 {
		URL, _ := url.Parse(uri)
		q := URL.Query()
		if req.OutletID > 0 {
			q.Add("outlet_id", fmt.Sprint(req.OutletID))
		}
		q.Add("limit", fmt.Sprint(req.Limit))
		q.Add("page", fmt.Sprint(req.Page+1))
		URL.RawQuery = q.Encode()
		link.NextPage = URL.String()
	}

	if req.Page > 1 && totalPage > 1 {
		URL, _ := url.Parse(uri)
		q := URL.Query()
		if req.OutletID > 0 {
			q.Add("outlet_id", fmt.Sprint(req.OutletID))
		}
		q.Add("limit", fmt.Sprint(req.Limit))
		q.Add("page", fmt.Sprint(req.Page-1))
		URL.RawQuery = q.Encode()
		link.PervPage = URL.String()
	}

	pagin := Pagination{
		Limit:     req.Limit,
		Page:      req.Page,
		TotalPage: int(totalPage),
	}

	return &TrxResponse{
		Pagination: pagin,
		Link:       link,
		Data:       res,
	}, nil
}

func (t *Transaction) Reporting(ctx context.Context, req *TrxRequest) (*TrxResponse, error) {
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Page <= 0 {
		req.Page = 1
	}

	if !req.IsValidRangeDate() {
		n := time.Now()
		y, m, _ := n.Date()
		start := time.Date(y, m, 1, 0, 0, 0, 0, n.Location())
		req.StartDate = start

		ey, em, ed := now.With(start).EndOfMonth().Date()
		end := time.Date(ey, em, ed, 23, 59, 59, 0, n.Location())
		req.EndDate = end
	}

	res, err := t.TransactionRepo.Reporting(ctx, transaction.ReportFilter{
		MerchantID: req.MerchantID,
		OutletID:   req.OutletID,
		StartDate:  req.StartDate,
		EndDate:    req.EndDate,
	})
	if err != nil {
		return nil, err
	}

	// start date
	start := now.With(req.StartDate)
	startEndDay := start.EndOfMonth().Day()

	// end date
	endYear, endMonth, _ := req.EndDate.Date()
	endLocation := req.EndDate.Location()

	days := int(math.Ceil(req.EndDate.Sub(req.StartDate).Hours() / 24))
	data := []schema.TransactionReport{}

	prevPage := (req.Page - 1) * req.Limit
	nextPage := req.Page * req.Limit

	// skiper a data response
	skip := 0

	day := start.BeginningOfMonth().Day()
	for i := 1; i <= days; i++ {
		// skip previous date
		if i < prevPage {
			continue
		}
		// stop loop when reach goal from current page
		if i > nextPage {
			break
		}

		var date time.Time
		if day <= startEndDay {
			// (i - 1) offset date control
			// # example :
			// 	day = 1;
			// 	i = 1;
			// 	calculated = day+i = 2 (expected start date 1)
			// 	must adding a offset control that is (i - 1)
			d := day + i - 1
			date = time.Date(start.Year(), start.Month(), d, 0, 0, 0, 0, start.Location())
		} else {
			// reset date to zero if start new month
			day = 0
			day++
			date = time.Date(endYear, endMonth, day, 0, 0, 0, 0, endLocation)
		}

		omzet := "0"
		// improve a looping data response with skipping
		for i, v := range res[skip:] {
			var (
				sameDay   = date.Day() == v.Date.Day()
				sameMonth = date.Month().String() == v.Date.Month().String()
			)

			// equalization day and month
			if sameDay && sameMonth {
				omzet = v.Omzet
				skip = i
				goto skipResLoop
			}
		}
	skipResLoop:

		data = append(data, schema.TransactionReport{
			Date:  date,
			Omzet: omzet,
		})
	}

	totalPage := math.Ceil(float64(days) / float64(req.Limit))
	if totalPage <= 0 {
		totalPage = 1
	}

	uri := "http://localhost:3000/reporting"
	URL, _ := url.Parse(uri)
	q := URL.Query()
	if req.OutletID > 0 {
		q.Add("outlet_id", fmt.Sprint(req.OutletID))
	}
	q.Add("limit", fmt.Sprint(req.Limit))
	q.Add("page", fmt.Sprint(req.Page))
	URL.RawQuery = q.Encode()

	link := Link{
		Current: URL.String(),
	}

	if totalPage > 1 {
		URL, _ := url.Parse(uri)
		q := URL.Query()
		if req.OutletID > 0 {
			q.Add("outlet_id", fmt.Sprint(req.OutletID))
		}
		q.Add("limit", fmt.Sprint(req.Limit))
		q.Add("page", fmt.Sprint(req.Page+1))
		URL.RawQuery = q.Encode()
		link.NextPage = URL.String()
	}

	if req.Page > 1 && totalPage > 1 {
		URL, _ := url.Parse(uri)
		q := URL.Query()
		if req.OutletID > 0 {
			q.Add("outlet_id", fmt.Sprint(req.OutletID))
		}
		q.Add("limit", fmt.Sprint(req.Limit))
		q.Add("page", fmt.Sprint(req.Page-1))
		URL.RawQuery = q.Encode()
		link.PervPage = URL.String()
	}

	pagin := Pagination{
		Limit:     req.Limit,
		Page:      req.Page,
		TotalPage: int(totalPage),
	}

	return &TrxResponse{
		Pagination: pagin,
		Link:       link,
		Data:       data,
	}, nil
}
