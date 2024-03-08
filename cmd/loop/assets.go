package main

import (
	"context"

	"github.com/lightninglabs/loop/looprpc"
	"github.com/urfave/cli"
)

var assetsCommands = cli.Command{

	Name:      "assets",
	ShortName: "a",
	Usage:     "manage asset swaps",
	Description: `
	`,
	Subcommands: []cli.Command{
		assetsOutCommand,
	},
}
var (
	assetsOutCommand = cli.Command{
		Name:      "out",
		ShortName: "o",
		Usage:     "swap asset out",
		ArgsUsage: "",
		Description: `
		List all reservations.
	`,
		Action: assetSwapOut,
	}
)

func assetSwapOut(ctx *cli.Context) error {
	// First set up the swap client itself.
	client, cleanup, err := getAssetsClient(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	res, err := client.SwapOut(
		context.Background(),
		&looprpc.SwapOutRequest{},
	)
	if err != nil {
		return err
	}

	printRespJSON(res)
	return nil
}

func getAssetsClient(ctx *cli.Context) (looprpc.AssetsClientClient, func(), error) {
	rpcServer := ctx.GlobalString("rpcserver")
	tlsCertPath, macaroonPath, err := extractPathArgs(ctx)
	if err != nil {
		return nil, nil, err
	}
	conn, err := getClientConn(rpcServer, tlsCertPath, macaroonPath)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { conn.Close() }

	loopClient := looprpc.NewAssetsClientClient(conn)
	return loopClient, cleanup, nil
}
