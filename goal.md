技术栈：Go，Echo/v5，Gorm+PostgreSQL，Redis，github.com/gorilla/feeds

我现在使用 RSSHub 作为 RSS 来源，但是从中获得条目有时标题不那么易读，我希望该项目作为一个网关：

1. 所有的流量都经过该网关
2. 如果路径被注册了处理器，那么处理并缓存到数据库中再返回
3. 如果路径没有注册处理器，不缓存，直接透穿
