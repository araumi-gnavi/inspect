package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	validGetCompareLog = false
)

func massiveSet(ctx context.Context, cli *redis.Client, db int, count int) error {
	semaphore := make(chan struct{}, 10)

	var wg sync.WaitGroup

	var err error

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(num int) {
			semaphore <- struct{}{}
			defer func() {
				wg.Done()
				<-semaphore
			}()

			e := cli.Set(ctx, fmt.Sprintf("key:%d", num), fmt.Sprintf("value-%d:%d", db, num), 0).Err()
			if e != nil {
				err = errors.Join(err, e)
			}
		}(i)
	}

	wg.Wait()

	return err
}

func massiveGet(ctx context.Context, cli *redis.Client, db int, count int) error {
	semaphore := make(chan struct{}, 10)

	var wg sync.WaitGroup

	var err error

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(num int) {
			semaphore <- struct{}{}
			defer func() {
				wg.Done()
				<-semaphore
			}()

			val, e := cli.Get(ctx, fmt.Sprintf("key:%d", num)).Result()
			if e != nil {
				err = errors.Join(err, e)
			}

			expect := fmt.Sprintf("value-%d:%d", db, num)
			if val != expect {
				if validGetCompareLog {
					log.Printf("expect:%s, actual:%s\n", expect, val)
				}
			}
		}(i)
	}

	wg.Wait()

	return err
}

func startHTTP(port int, cli *redis.Client) error {
	handler := func(w http.ResponseWriter, req *http.Request) {
		if strings.Contains(req.URL.Path, "favicon") {
			fmt.Fprintf(w, "favicon NG")
			return
		}

		switch req.URL.Path {
		case "/":
			fmt.Fprintf(w, "ok")
		case "/swap":
			err := req.ParseForm()
			if err != nil {
				log.Fatal("ParseForm err:", err)
			}

			db1str := req.Form.Get("db1")
			db2str := req.Form.Get("db2")

			db1, err := strconv.Atoi(db1str)
			if err != nil {
				log.Fatal("strconv err:", err)
			}

			db2, err := strconv.Atoi(db2str)
			if err != nil {
				log.Fatal("strconv err:", err)
			}

			conn := cli.Conn()

			defer conn.Close()

			err = conn.SwapDB(context.Background(), db1, db2).Err()
			if err != nil {
				log.Fatal("conn.SwapDB err:", err)
			}

			log.Println("SWAP!")

			fmt.Fprintf(w, "swap ok")
		case "/log":
			err := req.ParseForm()
			if err != nil {
				log.Fatal("ParseForm err:", err)
			}

			validGetCompareLog = !validGetCompareLog

			fmt.Fprintf(w, "log change ok. %v", validGetCompareLog)
		case "/get":
			err := req.ParseForm()
			if err != nil {
				log.Fatal("ParseForm err:", err)
			}

			key := req.Form.Get("key")

			res, err := cli.Get(context.Background(), key).Result()
			if err != nil {
				log.Fatal("redis client.Get err", err)
			}

			fmt.Fprintf(w, "get ok. %s", res)
		}

	}
	http.HandleFunc("/", handler)

	log.Println("START HTTP SERVER. port=", port)

	go func() {
		http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
	}()

	return nil
}

func main() {
	log.Println("START")
	defer log.Println("END")

	rdb0 := redis.NewClient(&redis.Options{
		Addr:     "redis:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	rdb1 := redis.NewClient(&redis.Options{
		Addr:     "redis:6379",
		Password: "", // no password set
		DB:       1,  // use default DB
	})

	rdb2 := redis.NewClient(&redis.Options{
		Addr:     "redis:6379",
		Password: "", // no password set
		DB:       2,  // use default DB
	})

	defer func() {
		err := rdb0.Close()
		if err != nil {
			log.Fatal(err)
		}

		err = rdb1.Close()
		if err != nil {
			log.Fatal(err)
		}

		err = rdb2.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()

	ctx := context.Background()

	err := massiveSet(ctx, rdb0, 0, 1000)
	if err != nil {
		log.Fatal(err)
	}

	err = massiveSet(ctx, rdb1, 1, 1000)
	if err != nil {
		log.Fatal(err)
	}

	err = massiveSet(ctx, rdb2, 2, 1000)
	if err != nil {
		log.Fatal(err)
	}

	startHTTP(8080, rdb0)

	go func() {
		for {
			err := massiveGet(ctx, rdb0, 0, 1000)
			if err != nil {
				log.Fatal(err)
			}

			err = massiveGet(ctx, rdb1, 1, 1000)
			if err != nil {
				log.Fatal(err)
			}

			err = massiveGet(ctx, rdb2, 2, 1000)
			if err != nil {
				log.Fatal(err)
			}

			runtime.Gosched()
		}
	}()

	for {
		val, err := rdb0.Get(ctx, "key:10").Result()
		if err != nil {
			log.Fatal(err)
		}

		log.Println("key:10=", val)

		time.Sleep(1 * time.Second)
	}
}
