// Package main is the entry point of the WattLedger API server.
// It connects to Hyperledger Fabric through the Fabric Gateway (one identity per
// consortium organization), simulates the IoT meters and the utility revenue
// meter, and exposes a REST API.
package main

import (
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"api/config"
	"api/gateway"
	"api/handlers"
	"api/iot"
	"api/services"
	"api/store"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

const serverPort = ":3000"

func main() {
	log.Println("Starting WattLedger API server...")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Println("Connecting to Hyperledger Fabric...")
	gw, err := gateway.New(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to Fabric: %v", err)
	}
	defer gw.Close()

	ledger := services.NewLedgerService(gw, cfg.Chaincodes.Governance, cfg.Chaincodes.Energy)

	simulator, err := iot.NewMeterSimulator(filepath.Join("data", "meters"))
	if err != nil {
		log.Fatalf("Failed to start meter simulator: %v", err)
	}

	userStore, err := store.NewUserStore(filepath.Join("data", "users.json"))
	if err != nil {
		log.Fatalf("Failed to open user store: %v", err)
	}

	router := setupRouter(
		handlers.NewNetworkHandler(gw, cfg.Channel),
		handlers.NewGovernanceHandler(ledger),
		handlers.NewMeterHandler(ledger, simulator),
		handlers.NewMeteringHandler(ledger, simulator),
		handlers.NewMarketHandler(ledger),
		handlers.NewUserHandler(userStore),
	)

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		log.Println("Shutting down...")
		gw.Close()
		os.Exit(0)
	}()

	log.Printf("Server listening on http://localhost%s", serverPort)
	if err := router.Run(serverPort); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func setupRouter(
	network *handlers.NetworkHandler,
	governance *handlers.GovernanceHandler,
	meters *handlers.MeterHandler,
	metering *handlers.MeteringHandler,
	market *handlers.MarketHandler,
	users *handlers.UserHandler,
) *gin.Engine {
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AllowHeaders = append(corsConfig.AllowHeaders, handlers.OrgHeader)
	router.Use(cors.New(corsConfig))

	api := router.Group("/api")
	{
		api.GET("/network", network.HandleNetworkInfo)

		gov := api.Group("/governance")
		{
			gov.GET("/members", governance.HandleListMembers)
			gov.POST("/members", governance.HandleOnboardMember)
			gov.GET("/tariffs", governance.HandleGetTariffs)
			gov.PUT("/tariffs", governance.HandleUpdateTariffs)
			gov.GET("/sandbox", governance.HandleGetSandbox)
			gov.PUT("/sandbox", governance.HandleSetSandbox)
			gov.GET("/price-band", governance.HandleGetPriceBand)
		}

		// Demonstrated scope: measurement, reconciliation, audit, simulated bill.
		m := api.Group("/meters")
		{
			m.GET("", meters.HandleListMeters)
			m.POST("", meters.HandleRegisterMeter)
			m.GET("/:meterID", meters.HandleGetMeter)
			m.POST("/:meterID/readings", meters.HandleSubmitReading)
			m.POST("/:meterID/deactivate", meters.HandleDeactivateMeter)
		}
		api.GET("/simulator/:meterID", metering.HandleSimulatorStatus)
		api.GET("/readings", metering.HandleListReadings)
		api.GET("/meter-states", metering.HandleListMeterStates)
		api.GET("/reconciliations", metering.HandleListReconciliations)
		api.POST("/reconciliations", metering.HandleReconcile)
		api.GET("/audit", metering.HandleAudit)
		api.POST("/billing/simulate", metering.HandleSimulateBill)
		api.GET("/billing/statements", metering.HandleListStatements)
		api.GET("/accounts", metering.HandleListAccounts)
		api.GET("/accounts/:owner", metering.HandleGetAccount)
		api.GET("/accounts/:owner/history", metering.HandleGetAccountHistory)

		// Future scope: P2P trading, refused unless the regulatory sandbox is on.
		sb := api.Group("/sandbox")
		{
			sb.GET("/orders", market.HandleListOrders)
			sb.POST("/asks", market.HandleSubmitAsk)
			sb.POST("/bids", market.HandleSubmitBid)
			sb.POST("/orders/:orderID/cancel", market.HandleCancelOrder)
			sb.POST("/match", market.HandleMatch)
			sb.GET("/trades", market.HandleListTrades)
		}

		u := api.Group("/users")
		{
			u.GET("", users.HandleListUsers)
			u.POST("", users.HandleCreateUser)
		}
	}
	return router
}
