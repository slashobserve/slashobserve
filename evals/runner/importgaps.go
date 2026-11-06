package main

import (
	"slices"
	"strings"
)

// Measured on the evals stack, which has no series for these panels' queries; a target among them fails the seed.
var unverifiedAtImport = map[string]string{
	"argo-cd/dashboard.json":                            "p34 p36 p42 p44 p46 p48 p49 p50 p61 p62 p71 p72 p84 p96 p99 p116 p34_2 p62_2 p108_2",
	"cilium/cilium-dashboard.json":                      "p24 p26 p33 p37 p39 p49 p51 p53 p55 p56 p68 p70 p76 p79 p81 p85 p87 p89 p91 p93 p96 p98 p100 p102 p104 p106 p108 p109 p111 p113 p114 p115 p117 p118 p119 p120 p121 p122 p123 p128 p129 p130 p140 p142 p143 p145 p146 p147 p148 p152 p153 p156 p157 p158 p159 p163 p165 p166 p168 p172 p173 p174 p175 p178 p184 p186 p188 p190 p192 p194",
	"clickhouse-datasource/otel-service-dashboard.json": "p1 p2 p3 p4 p6 p11 p12 p13 p14 p15 p16 p17 p18 p19 p20 p21 p22 p23",
	"rfmoz/haproxy.json":                                "p237",
	"rfmoz/node-exporter-full.json":                     "p128 p158 p297 p298 p300 p302 p305 p306 p321 p325 p331 p332 p341",
	"slashobserve/demo-shop.json":                       "p17",
	"victoriametrics/victoriametrics.json":              "p10 p12 p22 p26 p30 p34 p35 p36 p37 p38 p44 p47 p49 p51 p53 p57 p58 p59 p60 p62 p64 p66 p68 p73 p76 p77 p78 p81 p82 p90 p99 p103 p105 p106 p107 p108 p110 p112 p114 p116 p122 p123 p124 p125 p126 p127 p128 p129 p130 p131 p132 p133 p134 p135 p136 p137 p140 p141 p142 p143 p144 p145 p146 p147 p148 p149 p150 p151 p152 p153 p154 p155 p156 p159 p160",
	"victoriametrics/vmalert.json":                      "p4 p6 p7 p8 p9 p13 p14 p15 p20 p23 p24 p25 p26 p30 p31 p32 p33 p35 p37 p39 p41 p45 p48 p50 p52 p53 p54 p56 p57 p58 p59 p60 p61 p63 p64 p65 p66 p67 p68 p69 p70",
}

func verifiedAtImport(key, panelID string) bool {
	return !slices.Contains(strings.Fields(unverifiedAtImport[key]), panelID)
}
