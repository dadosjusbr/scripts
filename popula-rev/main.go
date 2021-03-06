package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/dadosjusbr/storage"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type config struct {
	MongoURI    string `envconfig:"MONGODB_URI" required:"true"`
	DBName      string `envconfig:"MONGODB_DBNAME" required:"true"`
	MongoMICol  string `envconfig:"MONGODB_MICOL" required:"true"`
	MongoAgCol  string `envconfig:"MONGODB_AGCOL" required:"true"`
	MongoPkgCol string `envconfig:"MONGODB_PKGCOL" required:"true"`
	MongoRevCol string `envconfig:"MONGODB_REVCOL" required:"true"`

	// Swift Conf
	SwiftUsername  string `envconfig:"SWIFT_USERNAME" required:"true"`
	SwiftAPIKey    string `envconfig:"SWIFT_APIKEY" required:"true"`
	SwiftAuthURL   string `envconfig:"SWIFT_AUTHURL" required:"true"`
	SwiftDomain    string `envconfig:"SWIFT_DOMAIN" required:"true"`
	SwiftContainer string `envconfig:"SWIFT_CONTAINER" required:"true"`
}

var (
	aid = flag.String("aid", "", "Órgão")
)

func main() {
	flag.Parse()

	if err := godotenv.Load(".env"); err != nil {
		log.Fatalf("Erro ao carregar arquivo .env: %v", err)
	}

	var c config
	if err := envconfig.Process("", &c); err != nil {
		log.Fatalf("Erro ao carregar parâmetros do arquivo .env:%v", err)
	}

	if *aid == "" {
		log.Fatal("Flag aid obrigatória")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(c.MongoURI))
	if err != nil {
		log.Fatal("mongo.Connect() ERROR:", err)
	}
	defer cancel()
	defer client.Disconnect(ctx)

	col := client.Database(c.DBName).Collection(c.MongoMICol)
	for year := 2018; year <= 2021; year++ {
		var operations []mongo.WriteModel
		filter := bson.M{
			"aid":  *aid,
			"year": year,
		}
		res, err := col.Find(ctx, filter)
		if err != nil {
			log.Fatalf("Erro ao consultar informações mensais do órgão: %v", err)
		}
		for res.Next(ctx) {
			var agmi storage.AgencyMonthlyInfo
			if err = res.Decode(&agmi); err != nil {
				log.Fatalf("[%s/%d/%d] Erro ao obter agmi: %s", agmi.AgencyID, agmi.Year, agmi.Month, err)
			}

			// ## Armazenando revisão.
			if agmi.ProcInfo == nil {
				fmt.Printf("%d/%d não ocorreu erro na coleta do %s\n", agmi.Month, agmi.Year, agmi.AgencyID)
				continue
			}
			rev := storage.MonthlyInfoVersion{
				AgencyID:  agmi.AgencyID,
				Month:     agmi.Month,
				Year:      agmi.Year,
				VersionID: agmi.CrawlingTimestamp.AsTime().Unix(),
				Version:   agmi,
			}
			operation := mongo.NewInsertOneModel().SetDocument(rev)
			operations = append(operations, operation)
		}
		if len(operations) > 0 {
			colRev := client.Database(c.DBName).Collection(c.MongoRevCol)
			results, err := colRev.BulkWrite(ctx, operations)
			if err != nil {
				log.Fatalf("Erro ao inserir em miRev [%s/%d]: %v", *aid, year, err)
			}
			fmt.Printf("Documentos inseridos: %d\n\n", results.InsertedCount)
		} else {
			fmt.Print("Não há documentos para inserir.\n\n")
		}
	}
}
