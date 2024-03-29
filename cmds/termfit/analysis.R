df<-read.csv("result.csv", header=F)
colnames(df)<-c("TTM","QuotedPrice","InitialRate","FinalRateNss","FinalRateSpline","CalculatedPriceNss","CalculatedPriceSpline", "zNss","zSpline")
df<-df[df$TTM>=2/12,]
max<-max(df[,c(3,4,5)])
min<-min(df[,c(3,4,5)])
maxP<-max(df[,c(6,7)])
minP<-min(df[,c(6,7)])
maxZ<-max(df[,c(8,9)])
minZ<-min(df[,c(8,9)])
save<-par(mfrow=c(3,1),mar=c(2,4,2,4))
x<-(df$TTM)
with(df, {
	plot(x, InitialRate,type="o",pch=3,ylim=c(min,max), main="Comparison of Term Structures", ylab="Spot rates in %", xlab="Time to maturity",log="x")
	lines(x, FinalRateNss, col="red", type="o",pch=3)
	lines(x, FinalRateSpline, col="green", type="o",pch=3)
	legend("topleft", legend=c("Starting Curve", "Nelson Siegel Svensson", "Cubic Splines"), lty=1, pch=3, col=c("black","red","green"), cex=1.0)
})
with(df, {
	plot(x, QuotedPrice,type="o",pch=3,ylim=c(minP,maxP), main="Comparison of Bond Prices", ylab="Bond prices in CHF", xlab="Time to maturity",log="x")
	lines(x, CalculatedPriceNss, col="red", type="o",pch=3)
	lines(x, CalculatedPriceSpline, col="green", type="o",pch=3)
	legend("topleft", legend=c("Quoted Price", "Nelson Siegel Svensson","Cubic Splines"), lty=1, pch=3, col=c("black","red", "green"),cex=1.0)
})
with(df, {
	plot(x, zNss,type="o",pch=3,col="red",ylim=c(minZ,maxZ), main="Comparison of Discount Factors Z", ylab="Discount factor", xlab="Time to maturity",log="x")
	lines(x, zSpline, col="green", type="o",pch=3)
	legend("topleft", legend=c("Nelson Siegel Svensson","Cubic Splines"), lty=1, pch=3,col=c("red", "green"),cex=1.0)
})
par(save)
