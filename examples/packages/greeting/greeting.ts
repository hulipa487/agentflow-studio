// af-prelude-version: 2
var Greeting = {
  hello(name?: string): string {
    return "hello, " + (name || "world");
  },
};
