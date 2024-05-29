# Wrap-Up and Further Reading

With that, we have a functioning app that we can easily extend to do more. There's still plenty of features of the SDK we didn't cover in this tutorial (to avoid making it even longer), but if you'd like to know more:

* [About Kinds](../../custom-kinds/README.md) is a deep dive into writing kinds and schemas, how to evolve a schema with semantic versioning, and what the different kind targets produce.
* [Operators and Event-Based Design](../../operators.md) talks about the functionality of the `operator` package, and how to do event-based design for your apps.

Another bit we didn't cover is that you can have your operator consume events for some other application's kinds and react to their changes--maybe when someone creates something in a different app, you want to make an Issue correlated to it. You can test if such a resource kind exists (basically, is the app installed?), and then create an informer controller to handle those events. Likewise, a different app might consume Issue events. 

This tutorial may also be periodically updated to add extra optional sections on implementing other bits of functionality.

### Prev: [Adding Admission Control](08-adding-admission-control.md)