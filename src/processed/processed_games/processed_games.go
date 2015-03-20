package main

import (
	raw "api/raw"
	"flag"
	"fmt"
	mgo "gopkg.in/mgo.v2"
	"log"
	"shared/structs"
	"time"
	// "github.com/iwanbk/gobeanstalk"
	// "log"
	gproto "code.google.com/p/goprotobuf/proto"
	proto "proto"
)

var MONGO_CONNECTION_URL = flag.String("mongodb", "localhost", "The URL that mgo should use to connect to Mongo.")

func addItem(c chan proto.ProcessedJobRequest) {
	c <- proto.ProcessedJobRequest{
		//		Type:     gproto.Enum(proto.ProcessedJobRequest_GENERATE_PROCESSED_GAME),
		TargetId: gproto.Int64(1748100615),
	}

	close(c)
}

func getJobChannel() chan proto.ProcessedJobRequest {
	jc := make(chan proto.ProcessedJobRequest)
	go addItem(jc)

	return jc
}

func main() {
	flag.Parse()
	// TODO: replace this with pulling something from a live queue.
	jc := getJobChannel()

	for job := range jc {
		fmt.Println(fmt.Sprintf("Recceived job PROCESS GAME: %d", *job.TargetId))

		pg := structs.ProcessedGame{}
		pg.GameId = int(*job.TargetId)

		// Fetch all instances of raw games that have information about
		// this game ID and store them.
		gr := raw.GetPartialGames(pg.GameId)
		pps := make(map[int]structs.ProcessedPlayerStats)

		for _, response := range gr {
			for _, game := range response.Games {
				pg.GameTimestamp = int64(game.CreateDate)
				// Divide by one thousand since the value is in milliseconds.
				pg.GameDate = time.Unix(int64(game.CreateDate)/1000, 0).Format("2006-01-02")
				// Only do processing on the game that's being handled in this job.
				// Other games should be discarded.
				if game.GameId == pg.GameId {
					fmt.Println(response.SummonerId)
					// TODO: instead of getting 'latest', should get 'closest to timestamp X (but not after)'.
					// Current approach works fine unless we're running a backfill.
					latestLeague, lerr := raw.GetLatestLeagues(response.SummonerId, "RANKED_SOLO_5x5")
					tier := "UNKNOWN"
					division_str := "0"
					division := 0

					if lerr == nil {
						// Sort through all of the entries and find one of the requested participant.
						for _, entry := range latestLeague.Entries {
							if entry.PlayerOrTeamId == latestLeague.ParticipantId {
								tier = latestLeague.Tier
								division_str = entry.Division
							}
						}

						// Convert the
						switch division_str {
						case "I":
							division = 1
							break
						case "II":
							division = 2
							break
						case "III":
							division = 3
							break
						case "IV":
							division = 4
							break
						case "V":
							division = 5
							break
						default:
							division = 0
							break
						}
					}
					// This GameRecord has enough information to populate one user's
					// ProcessedPlayerStats. Generate that object, add it to the game,
					// and look for others.
					pps[response.SummonerId] = structs.ProcessedPlayerStats{
						SummonerId:       response.SummonerId,
						SummonerTier:     tier,     // TODO: fetch this from raw_leagues logs
						SummonerDivision: division, // TODO: fetch this from raw_leagues logs
						NumDeaths:        game.Stats.NumDeaths,
						MinionsKilled:    game.Stats.MinionsKilled,
						WardsPlaced:      game.Stats.WardPlaced,
						WardsCleared:     game.Stats.WardKilled,
					}
				}
			}
		}

		// Copy one PPS entry per summoner into the processed game file.
		for _, v := range pps {
			pg.Stats = append(pg.Stats, v)
		}

		// Create a MongoDB session and save the data.
		log.Println("Connecting to Mongo @ " + *MONGO_CONNECTION_URL)
		session, cerr := mgo.Dial(*MONGO_CONNECTION_URL)
		if cerr != nil {
			fmt.Println("Cannot connect to mongodb instance")
			return
		}
		collection := session.DB("league").C("processed_games")
		log.Println(fmt.Sprintf("Saving processed game #%d...", pg.GameId))
		collection.Insert(pg)
		log.Println("Done.")
	}

	// conn, err := gobeanstalk.Dial("localhost:11300")
	// if err != nil {
	// 	log.Fatal(err.Error())
	// }

	// for {
	// 	raw_job, rerr := conn.Reserve()
	// 	if rerr != nil {
	// 		log.Fatal(rerr.Error())
	// 	}

	// 	job := raw_job.(proto.ProcessedJobRequest)

	// }
}
