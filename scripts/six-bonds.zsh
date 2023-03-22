#!/bin/zsh

# BONDS FROM SIX EXCHANGE
# https://www.six-group.com/fqs/ref.csv?select=MaturityDate,CouponRate,ClosingPrice,SpecialFlagDesc&where=PortalSegment=BO*TradingBaseCurrency=CHF*ValorSymbol_~GKB&orderby=MaturityDate&page=1&pagesize=99999
#
# HEADERS
#
# ShortName
# ISIN
# ClosingPrice
# CouponRate
# IssuerNameShort
# ValorSymbol
# ValorNumber
# YieldToWorst
# DurationToWorst
# SubscriptionPaymentDueDate
# RemainingTermOfMaturity
# MaturityDate
# AmountInIssue
# ProductLineDesc
# TradingBaseCurrency
# ClosingPerformance
# ClosingDelta
# BidVolume
# BidPrice
# AskPrice
# AskVolume
# MidSpread
# PreviousClosingPrice
# LatestTradeDate
# LatestTradeTime
# OpeningPrice
# DailyHighPrice
# DailyLowPrice
# OnMarketVolume
# OffBookVolume
# TotalTurnover
# TotalTurnoverCHF
# GeographicalAreaDesc
# IndustrySectorDesc
# SecTypeDesc
# BondListedFlag
# BondDutyToReportFlag
# SpecialFlagDesc&where=PortalSegment=BO*TradingBaseCurrency=CHF*ValorSymbol_~GKB&orderby=MaturityDate&page=1&pagesize=99999
#
# SARON 3M: https://www.six-group.com/exchanges/downloads/indexdata/h_sar3mc_delayed.csv

ISSUER="$1" 
LINK="https://www.six-group.com/fqs/ref.csv?select=MaturityDate,CouponRate,ClosingPrice,IssuerNameShort,SpecialFlagDesc&where=PortalSegment=BO*TradingBaseCurrency=CHF*IssuerNameShort_~${ISSUER:-Glarner}&orderby=MaturityDate&page=1&pagesize=99999"

curl "$LINK" | awk 'NR>1' | while read line 
do 
	# maturity date, coupon, price
	echo $line | awk -F';' '{printf("%s-%s-%s,%3.2f,%3.2f,%s\n",substr($1,1,4),substr($1,5,2),substr($1,7,2), $2, $3, $4)}'
done 

