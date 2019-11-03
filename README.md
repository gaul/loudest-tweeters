# loudest-tweeters

loudest-tweeters shows all the [Twitter](https://twitter.com/) profiles in your
timeline, sorted by number of tweets.  This identifies the loudest ones which
you can silence by manually disabling retweets or muting.

## Usage

loudest-tweeters requires a
[Twitter API access token](https://developer.twitter.com/en/docs/basics/authentication/guides/access-tokens.html).
First get the source via `go get github.com/gaul/loudest-tweeters`.  Then
compile via `go build`.  Finally run via:

```
 TWITTER_KEY=xxx TWITTER_SECRET=xxx TWITTER_ACCESS_TOKEN=xxx TWITTER_ACCESS_TOKEN_SECRET=xxx ./loudest-tweeters
```

## TODO

* allow muting via inline hyperlink
* compile against anaconda 1.0.0
* run in AWS Lambda

## License

Copyright (C) 2017-2019 Andrew Gaul

Licensed under the MIT License
