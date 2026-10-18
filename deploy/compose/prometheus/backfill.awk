function load(t) {
    return 1 - 0.2 * (1 - cos(2 * 3.14159265 * (t - stop) / 86400))
}

BEGIN {
    step = 30
    fine_step = 5
    ramp = 0.0003
    start = now - 26 * 3600
    stop = now

    nt = 0
    for (t = stop; t >= start; t -= gap) {
        back[++nt] = t
        gap = fine_step * exp(ramp * (stop - t))
        if (gap > step) gap = step
    }
    for (i = 1; i <= nt; i++) ts[i] = back[nt - i + 1]

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

    nb = split("0.005 0.01 0.025 0.05 0.1 0.25 0.5 1.0 2.5 5.0 10.0", le, " ")
    split("0 0.005 0.01 0.025 0.05 0.1 0.25 0.5 1 2.5 5 10", bound, " ")
    profile["frontend|/api/products"] = "2:0.16 3:0.60 4:0.23 5:0.01"
    profile["frontend|/api/products/{id}"] = "2:0.164 3:0.588 4:0.228 5:0.02"
    profile["frontend|/api/checkout"] = "1:0.006 2:0.018 5:0.484 6:0.442 7:0.05"
    profile["frontend|/api/orders/{id}"] = "2:0.015 3:0.520 4:0.436 5:0.03"
    profile["checkout|/checkout"] = "1:0.024 5:0.615 6:0.311 7:0.05"
    profile["checkout|/orders/{id}"] = "2:0.162 3:0.582 4:0.226 5:0.03"
    profile["payments|/charge"] = "4:0.458 5:0.492 6:0.05"

    ng = 0
    for (i = 1; i <= n; i++) {
        split(series[i], f, "|")
        group = f[1] "|" f[2] "|" f[3]
        if (!(group in groupRate)) groups[++ng] = group
        groupRate[group] += f[5]
    }

    print "# TYPE http_requests counter"
    for (i = 1; i <= n; i++) {
        split(series[i], f, "|")
        labels = sprintf("{job=\"shop\",instance=\"%s:8080\",service=\"%s\",route=\"%s\",status=\"%s\",pod=\"%s\"}", f[1], f[2], f[3], f[4], f[1])
        value = 0
        for (j = 1; j <= nt; j++) {
            t = ts[j]
            printf "http_requests_total%s %.3f %.3f\n", labels, value, t
            if (j < nt) value += f[5] * load(t) * (ts[j + 1] - t)
        }
    }
    print "# TYPE http_request_duration_seconds histogram"
    for (g = 1; g <= ng; g++) {
        split(groups[g], f, "|")
        pod = f[1]; service = f[2]; route = f[3]
        key = service "|" route
        if (!(key in profile)) {
            print "backfill.awk: no latency profile for " key > "/dev/stderr"
            exit 1
        }
        for (b = 1; b <= nb; b++) weight[b] = 0
        top = 0
        np = split(profile[key], parts, " ")
        for (p = 1; p <= np; p++) {
            split(parts[p], kv, ":")
            weight[kv[1] + 0] = kv[2] + 0
            if (kv[1] + 0 > top) top = kv[1] + 0
        }
        labels = sprintf("job=\"shop\",instance=\"%s:8080\",service=\"%s\",route=\"%s\",pod=\"%s\"", pod, service, route, pod)

        amplitude = 1 - 0.2 * substr(pod, length(pod))
        for (b = 1; b <= nb; b++) bucket[b] = 0
        total = 0
        sum = 0
        for (k = 1; k <= nt; k++) {
            t = ts[k]
            time[k] = t
            if (k > 1) {
                count = groupRate[groups[g]] * load(t) * (t - ts[k - 1])
                tail = amplitude * (1 - cos(2 * 3.14159265 * (t - stop) / 86400))
                norm = 1 + weight[top] * (tail - 1)
                for (b = 1; b <= nb; b++) {
                    share = weight[b] * (b == top ? tail : 1) / norm
                    bucket[b] += count * share
                    sum += count * share * (bound[b] + bound[b + 1]) / 2
                }
                total += count
            }
            running = 0
            for (b = 1; b <= nb; b++) {
                running += bucket[b]
                cumulative[b, k] = running
            }
            totals[k] = total
            sums[k] = sum
        }
        samples = nt

        for (b = 1; b <= nb; b++) {
            for (k = 1; k <= samples; k++) {
                printf "http_request_duration_seconds_bucket{%s,le=\"%s\"} %.6f %.3f\n", labels, le[b], cumulative[b, k], time[k]
            }
        }
        for (k = 1; k <= samples; k++) {
            printf "http_request_duration_seconds_bucket{%s,le=\"+Inf\"} %.6f %.3f\n", labels, totals[k], time[k]
        }
        for (k = 1; k <= samples; k++) {
            printf "http_request_duration_seconds_sum{%s} %.6f %.3f\n", labels, sums[k], time[k]
        }
        for (k = 1; k <= samples; k++) {
            printf "http_request_duration_seconds_count{%s} %.6f %.3f\n", labels, totals[k], time[k]
        }
    }
    print "# EOF"
}
