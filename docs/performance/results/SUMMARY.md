# Benchmark 中位数（5 次）

Go 1.23.1，Apple M3 Max / darwin arm64，GOMAXPROCS=1。时间是局部操作耗时，不是整机 CPU 降幅。首次连接/dispatcher 使用预编译后二次交替复测结果。

| 场景 | before ns/op | after ns/op | 耗时减少 | before B/op | after B/op | before allocs/op | after allocs/op |
| -- | --: | --: | --: | --: | --: | --: | --: |
| GetUserBucket/users=100/tcp=true | 498.40 | 107.40 | 78.5% | 504 | 0 | 14 | 0 |
| GetUserBucket/users=100/tcp=false | 130.20 | 64.19 | 50.7% | 96 | 0 | 2 | 0 |
| GetUserBucket/users=1000/tcp=true | 558.10 | 115.10 | 79.4% | 504 | 0 | 14 | 0 |
| GetUserBucket/users=1000/tcp=false | 136.80 | 66.75 | 51.2% | 96 | 0 | 2 | 0 |
| GetUserBucket/users=10000/tcp=true | 771.60 | 127.70 | 83.4% | 504 | 0 | 14 | 0 |
| GetUserBucket/users=10000/tcp=false | 168.00 | 75.29 | 55.2% | 96 | 0 | 2 | 0 |
| GetUserBucket/users=50000/tcp=true | 1,156.00 | 175.50 | 84.8% | 505 | 0 | 14 | 0 |
| GetUserBucket/users=50000/tcp=false | 211.50 | 90.41 | 57.3% | 96 | 0 | 2 | 0 |
| BucketFirstConnection | 985.10 | 961.50 | 2.4% | 1,104 | 1,104 | 24 | 24 |
| BucketParallel | 452.50 | 66.13 | 85.4% | 504 | 0 | 14 | 0 |
| CompareUserList/users=100/unchanged | 61,674.00 | 642.00 | 99.0% | 90,009 | 0 | 26 | 0 |
| CompareUserList/users=100/one-percent | 63,893.00 | 23,136.00 | 63.8% | 90,351 | 15,483 | 28 | 5 |
| CompareUserList/users=100/all | 90,670.00 | 55,957.00 | 38.3% | 153,009 | 110,203 | 39 | 24 |
| CompareUserList/users=1000/unchanged | 717,938.00 | 6,457.00 | 99.1% | 1,173,489 | 0 | 79 | 0 |
| CompareUserList/users=1000/one-percent | 722,093.00 | 238,527.00 | 67.0% | 1,179,945 | 227,928 | 89 | 12 |
| CompareUserList/users=1000/all | 1,023,086.00 | 569,983.00 | 44.3% | 1,871,772 | 1,156,295 | 122 | 55 |
| CompareUserList/users=10000/unchanged | 7,049,236.00 | 68,876.00 | 99.0% | 11,701,418 | 0 | 579 | 0 |
| CompareUserList/users=10000/one-percent | 7,294,918.00 | 2,402,946.00 | 67.1% | 11,768,903 | 1,794,445 | 599 | 20 |
| CompareUserList/users=10000/all | 10,645,862.00 | 6,433,021.00 | 39.6% | 19,681,752 | 13,975,135 | 894 | 334 |
| CompareUserList/users=50000/unchanged | 38,125,840.00 | 342,447.00 | 99.1% | 56,284,176 | 0 | 4,030 | 0 |
| CompareUserList/users=50000/one-percent | 39,071,430.00 | 14,055,456.00 | 64.0% | 56,596,666 | 7,946,560 | 4,108 | 901 |
| CompareUserList/users=50000/all | 60,901,948.00 | 36,646,868.00 | 39.8% | 96,971,216 | 73,392,557 | 6,094 | 2,909 |
| RuleDetect/rules=1/hit=false | 62.00 | 35.02 | 43.5% | 24 | 0 | 1 | 0 |
| RuleDetect/rules=1/hit=true | 369.10 | 179.80 | 51.3% | 344 | 64 | 8 | 2 |
| RuleDetect/rules=20/hit=false | 974.30 | 463.10 | 52.5% | 480 | 0 | 20 | 0 |
| RuleDetect/rules=20/hit=true | 1,364.00 | 747.60 | 45.2% | 800 | 64 | 27 | 2 |
| DispatcherGetLink | 1,552.00 | 1,137.00 | 26.7% | 2,096 | 1,592 | 48 | 34 |
| CompareUserList/users=10000/reordered | 7,395,651.00 | 2,351,161.00 | 68.2% | 11,702,552 | 1,729,089 | 580 | 2 |
