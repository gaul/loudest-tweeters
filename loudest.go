package main

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/ChimeraCoder/anaconda"
)

type userStatistics struct {
	user       anaconda.User
	tweets     int
	retweets   int
	muted      bool
	noRetweets bool
	oldest     time.Time
}

type cachedUserStatistics struct {
	stats     map[int64]userStatistics
	cacheTime int64
}

const RATE_LIMIT_TIMEOUT = 15 * 60

var api *anaconda.TwitterApi

var tokensToSecrets = make(map[string]string)

var cachedTimelines = make(map[string]cachedUserStatistics)

func getCachedTimeline(api *anaconda.TwitterApi) (map[int64]userStatistics, error) {
	now := time.Now().Unix()
	if stats, ok := cachedTimelines[api.Credentials.Token]; ok && stats.cacheTime-RATE_LIMIT_TIMEOUT < now {
		return stats.stats, nil
	}

	stats, err := getUncachedTimeline(api)
	if err != nil {
		return nil, err
	}

	cachedTimelines[api.Credentials.Token] = cachedUserStatistics{stats, now}

	return stats, nil
}

func getUncachedTimeline(api *anaconda.TwitterApi) (map[int64]userStatistics, error) {
	stats := make(map[int64]userStatistics)
	maxID := int64(math.MaxInt64)
	for {
		values := url.Values{
			"count":            []string{"200"},
			"include_entities": []string{"false"},
			"exclude_replies":  []string{"false"},
		}
		if maxID != math.MaxInt64 {
			values["max_id"] = []string{fmt.Sprintf("%d", maxID)}
		}
		tweets, err := api.GetHomeTimeline(values)
		if err != nil {
			return nil, err
		}
		if len(tweets) == 0 {
			break
		}

		for _, tweet := range tweets {
			stat, ok := stats[tweet.User.Id]
			if !ok {
				stat = userStatistics{
					user:   tweet.User,
					oldest: time.Now(),
				}
			}
			stat.tweets += 1
			if tweet.RetweetedStatus != nil {
				stat.retweets += 1
			}
			createdAt, err := tweet.CreatedAtTime()
			if err != nil {
				return nil, err
			} else if !createdAt.IsZero() && stat.oldest.After(createdAt) {
				stat.oldest = createdAt
			}
			maxID = tweet.Id - 1
			stats[tweet.User.Id] = stat
		}
	}
	return stats, nil
}

func parseTimeline(api *anaconda.TwitterApi, stats map[int64]userStatistics) error {
	users, err := getFriends(api)
	if err != nil {
		return err
	}
	for _, user := range users {
		if _, ok := stats[user.Id]; !ok {
			stats[user.Id] = userStatistics{
				user: user,
			}
		}
	}

	muted, err := getMuted(api)
	if err != nil {
		return err
	}
	for _, id := range muted {
		if stat, ok := stats[id]; ok {
			stat.muted = true
			stats[id] = stat
		}
	}

	noRetweets, err := getNoRetweets(api)
	if err != nil {
		return err
	}
	for _, id := range noRetweets {
		if stat, ok := stats[id]; ok {
			stat.noRetweets = true
			stats[id] = stat
		}
	}

	return nil
}

func getFriends(api *anaconda.TwitterApi) ([]anaconda.User, error) {
	var stats []anaconda.User
	nextCursor := int64(-1)
	for {
		cursor, err := api.GetFriendsList(url.Values{
			"count":  []string{"200"},
			"cursor": []string{fmt.Sprintf("%d", nextCursor)},
		})
		if err != nil {
			return nil, err
		}
		stats = append(stats, cursor.Users...)
		if cursor.Next_cursor == -1 {
			break
		}
		nextCursor = cursor.Next_cursor
	}
	return stats, nil
}

func getMuted(api *anaconda.TwitterApi) ([]int64, error) {
	var muted []int64
	nextCursor := int64(-1)
	for {
		cursor, err := api.GetMutedUsersIds(url.Values{
			"cursor": []string{fmt.Sprintf("%d", nextCursor)},
		})
		if err != nil {
			return nil, err
		}
		muted = append(muted, cursor.Ids...)
		if cursor.Next_cursor == -1 {
			break
		}
		nextCursor = cursor.Next_cursor
	}
	return muted, nil
}

func getNoRetweets(api *anaconda.TwitterApi) ([]int64, error) {
	noRetweets, err := api.GetFriendshipsNoRetweets()
	if err != nil {
		return nil, err
	}
	return noRetweets, nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	log.Printf("request: %+v\n", r)

	if r.Method == "GET" && r.URL.Path == "/prune.png" {
		w.Header().Add("Content-Type", "image/png")
		http.ServeFile(w, r, "prune.png")
		return
	} else if r.Method != "GET" && (r.URL.Path != "" || r.URL.Path != "/") {
		http.Error(w, "Unknown endpoint", http.StatusForbidden)
		return
	}

	api.ReturnRateLimitError(true)

	stats, err := getCachedTimeline(api)
	if err != nil {
		if aErr, ok := err.(*anaconda.ApiError); ok {
			if isRateLimitError, nextWindow := aErr.RateLimitCheck(); isRateLimitError {
				timeoutMinutes := (nextWindow.Unix() - time.Now().Unix() + 59) / 60
				message := fmt.Sprintf("Twitter API rate limit exceeded - try again in %d minutes.", timeoutMinutes)
				http.Error(w, message, http.StatusTooManyRequests)
				return
			}
		}
		http.Error(w, fmt.Sprintf("Error getting timeline: %+v", err), http.StatusForbidden)
		return
	}

	err = parseTimeline(api, stats)
	if err != nil {
		// TODO: duplicating logic
		if aErr, ok := err.(*anaconda.ApiError); ok {
			if isRateLimitError, nextWindow := aErr.RateLimitCheck(); isRateLimitError {
				timeoutMinutes := (nextWindow.Unix() - time.Now().Unix() + 59) / 60
				message := fmt.Sprintf("Twitter API rate limit exceeded - try again in %d minutes.", timeoutMinutes)
				http.Error(w, message, http.StatusTooManyRequests)
				return
			}
		}
		http.Error(w, fmt.Sprintf("Error parsing timeline: %+v", err), http.StatusForbidden)
		return
	}

	w.Header().Add("Content-Type", "text/html")

	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
	<title>Loudest Tweeters</title>
	<link rel="icon" type="image/png" href="prune.png" sizes="32x32" />
	<link rel="stylesheet" type="text/css" href="//cdn.datatables.net/1.10.15/css/jquery.dataTables.css">
	<style type="text/css">
		body {
			margin-left: auto;
			margin-right: auto;
			max-width: 70ex;
		}
	</style>
	<script src="//code.jquery.com/jquery-1.12.4.js"></script>
	<script type="text/javascript" charset="utf8" src="//cdn.datatables.net/1.10.15/js/jquery.dataTables.js"></script>
	<script>
		$(document).ready( function () {
			$('#table_id').DataTable( {
				"bPaginate": false,
				"bInfo": false,
				"order": [[ 1, "desc" ]],
				"searching": false
			} );
		} );
	</script>
</head>
<body>
<p>Identify noisy accounts in your Twitter timeline.
Prune by disabling retweets or muting tweets.</p>
<table id="table_id" class="display">
<thead><tr><th>Screen name</th><th>Tweets</th><th>Retweets</th><th>Status</th></tr></thead>
`)
	oldest := time.Now()
	for _, stat := range stats {
		extra := ""
		if stat.muted {
			extra = "muted"
		} else if stat.noRetweets {
			extra = "no retweets"
		}
		fmt.Fprintf(w, `<tr>
	<td><a href="https://twitter.com/%s">@%s</a></td>
	<td>%d</td>
	<td>%d</td>
	<td>%s</td>
</tr>
`, stat.user.ScreenName, stat.user.ScreenName, stat.tweets, stat.retweets, extra)
		if !stat.oldest.IsZero() && oldest.After(stat.oldest) {
			oldest = stat.oldest
		}
	}
	fmt.Fprintf(w, `</table>
<p>Oldest Tweet: %s</p>
<p>Results may lag 15 minutes due to Twitter API rate limits.</p>
</body>
</html>
`, oldest)
}

func main() {
	consumerKey := os.Getenv("TWITTER_KEY")
	consumerSecret := os.Getenv("TWITTER_SECRET")
	accessToken := os.Getenv("TWITTER_ACCESS_TOKEN")
	accessTokenSecret := os.Getenv("TWITTER_ACCESS_TOKEN_SECRET")
	if consumerKey == "" || consumerSecret == "" || accessToken == "" || accessTokenSecret == "" {
		log.Println("Must set TWITTER_KEY, TWITTER_SECRET, TWITTER_ACCESS_TOKEN, and TWITTER_ACCESS_TOKEN_SECRET environment variables")
		os.Exit(1)
	}
	api = anaconda.NewTwitterApiWithCredentials(accessToken, accessTokenSecret, consumerKey, consumerSecret)

	addr := "localhost:8080"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}
	http.HandleFunc("/", handler)
	log.Printf("Listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
