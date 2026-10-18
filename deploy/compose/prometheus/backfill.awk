BEGIN {
    step = 30
    start = now - 26 * 3600
    stop = now

    n = 0
    series[++n] = "frontend-0|frontend|/api/products|200|10.0"
    series[++n] = "frontend-0|frontend|/api/products/{id}|200|4.5"
    series[++n] = "frontend-0|frontend|/api/products/{id}|404|0.5"
    series[++n] = "frontend-0|frontend|/api/checkout|201|3.7"
    series[++n] = "frontend-0|frontend|/api/checkout|400|0.12"
    series[++n] = "frontend-0|frontend|/api/checkout|402|0.08"
    series[++n] = "frontend-0|frontend|/api/orders/{id}|200|1.0"
    for (pod = 0; pod < 2; pod++) {
        series[++n] = "checkout-" pod "|checkout|/checkout|201|1.85"
        series[++n] = "checkout-" pod "|checkout|/checkout|400|0.06"
        series[++n] = "checkout-" pod "|checkout|/checkout|402|0.04"
        series[++n] = "checkout-" pod "|checkout|/orders/{id}|200|0.5"
    }
    series[++n] = "payments-0|payments|/charge|200|3.8"
    series[++n] = "payments-0|payments|/charge|402|0.08"

    print "# TYPE http_requests counter"
    for (i = 1; i <= n; i++) {
        split(series[i], f, "|")
        labels = sprintf("{job=\"shop\",instance=\"%s:8080\",service=\"%s\",route=\"%s\",status=\"%s\",pod=\"%s\"}", f[1], f[2], f[3], f[4], f[1])
        value = 0
        for (t = start; t <= stop; t += step) {
            printf "http_requests_total%s %.3f %d\n", labels, value, t
            value += f[5] * (1 + 0.25 * sin(2 * 3.14159265 * (t % 86400) / 86400)) * step
        }
    }
    print "# EOF"
}
