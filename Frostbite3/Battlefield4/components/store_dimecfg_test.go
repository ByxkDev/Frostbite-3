package components

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"
)

func TestShippedDimeCfg(t *testing.T) {
	p := filepath.Join("..", CDNDir, "dice-commerce/battlefield4/editorial/prod/20151027-01/store/gen3/ps3/eu/dimecfg.xml")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Version     int `xml:"configFormatVersion"`
		Consumables []struct {
			Name string `xml:"name"`
		} `xml:"content>consumables>consumable"`
	}
	if err := xml.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if v.Version != 1 || len(v.Consumables) != 8 {
		t.Fatalf("version %d, %d consumables", v.Version, len(v.Consumables))
	}
}

func TestShippedStoreItems(t *testing.T) {
	p := filepath.Join("..", CDNDir, "dice-commerce/battlefield4/editorial/prod/20151027-01/store/gen3/ps3/storeitems_en.json")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var items []struct {
		ID             int    `json:"id"`
		LinkIdentifier string `json:"linkIdentifier"`
		CategoryID     string `json:"categoryId"`
	}
	if err := json.Unmarshal(b, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 39 || items[0].LinkIdentifier != "premiumcategory" || items[38].ID != 232 {
		t.Fatalf("%d items", len(items))
	}
}
