// Orders service: lists a customer's recent orders with their items.
package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Item struct {
	SKU   string `json:"sku"`
	Qty   int    `json:"qty"`
	Price int    `json:"price_cents"`
}

type Order struct {
	ID    int64  `json:"id"`
	Total int    `json:"total_cents"`
	Items []Item `json:"items"`
}

var db *sql.DB

func main() {
	var err error
	db, err = sql.Open("pgx", "postgres://app@db/orders")
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(20)
	db.SetConnMaxIdleTime(5 * time.Minute)

	http.HandleFunc("/orders", listOrders)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	srv := &http.Server{Addr: ":8080", ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

// listOrders returns the customer's 20 most recent orders, each with its items.
func listOrders(w http.ResponseWriter, r *http.Request) {
	customer := r.URL.Query().Get("customer")
	rows, err := db.QueryContext(r.Context(),
		`SELECT id, total_cents FROM orders WHERE customer_id = $1 ORDER BY created_at DESC LIMIT 20`, customer)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.Total); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		orders = append(orders, o)
	}
	for i := range orders {
		items, err := db.QueryContext(r.Context(),
			`SELECT sku, qty, price_cents FROM order_items WHERE order_id = $1`, orders[i].ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for items.Next() {
			var it Item
			if err := items.Scan(&it.SKU, &it.Qty, &it.Price); err != nil {
				items.Close()
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			orders[i].Items = append(orders[i].Items, it)
		}
		items.Close()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}
