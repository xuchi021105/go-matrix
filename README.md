# go-matrix
由于看到大佬朋友用awk在终端里面实现了黑客帝国中代码雨的效果,所以想用go语言也实现一个,之后在GitHub上找到了[gomatrix](https://github.com/GeertJohan/gomatrix),查看源代码并借鉴思路仿写了一个(使用了go的tcell库)(gomatrix是用goroutine来控制雨的,而我仿写的是用一个for循环来集中同步控制),具体细节可以查看源代码中的注释 :-)

## 效果图

![Image](https://github.com/xuchi021105/Images/blob/main/go-matrix/matrix.gif)