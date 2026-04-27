package queue

import (
	"context"
	"net/http"

	"station-backend/pkg/utils"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type InitResponse struct {
	Station      InitStation      `json:"station"`
	Company      InitCompany      `json:"company"`
	Destinations []InitDest       `json:"destinations"`
}

type InitStation struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Governorate  string  `json:"governorate"`
	Delegation   string  `json:"delegation"`
	Address      string  `json:"address"`
	OpeningTime  string  `json:"openingTime"`
	ClosingTime  string  `json:"closingTime"`
	ServiceFee   float64 `json:"serviceFee"`
}

type InitCompany struct {
	Name    string `json:"name"`
	LogoURL string `json:"logoUrl"`
}

type InitDest struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	BasePrice float64 `json:"basePrice"`
	ServiceFee float64 `json:"serviceFee"`
}

type InitHandler struct {
	db *pgxpool.Pool
}

func NewInitHandler(db *pgxpool.Pool) *InitHandler {
	return &InitHandler{db: db}
}

func (h *InitHandler) GetInit(c *gin.Context) {
	ctx := context.Background()
	var resp InitResponse

	row := h.db.QueryRow(ctx, `
		SELECT station_id, station_name, governorate, delegation,
		       COALESCE(address, ''), opening_time, closing_time, service_fee,
		       COALESCE(company_name, ''), COALESCE(company_logo_url, '')
		FROM station_config
		WHERE is_operational = true
		LIMIT 1`)
	err := row.Scan(
		&resp.Station.ID, &resp.Station.Name, &resp.Station.Governorate, &resp.Station.Delegation,
		&resp.Station.Address, &resp.Station.OpeningTime, &resp.Station.ClosingTime, &resp.Station.ServiceFee,
		&resp.Company.Name, &resp.Company.LogoURL,
	)
	if err != nil {
		resp.Station = InitStation{Name: "Station"}
		resp.Company = InitCompany{Name: "Wasla"}
	}

	rows, err := h.db.Query(ctx, `
		SELECT station_id, station_name, base_price, COALESCE(service_fee, 0.200)
		FROM routes
		WHERE is_active = true
		ORDER BY station_name`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var d InitDest
			if err := rows.Scan(&d.ID, &d.Name, &d.BasePrice, &d.ServiceFee); err == nil {
				resp.Destinations = append(resp.Destinations, d)
			}
		}
	}
	if resp.Destinations == nil {
		resp.Destinations = []InitDest{}
	}

	utils.SuccessResponse(c, http.StatusOK, "Init data", resp)
}
