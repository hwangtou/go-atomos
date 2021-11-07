# 欢迎来到Atomos的世界！

## 关于项目

Atomos是什么？

* Atomos开发的初衷，是实现一套易于开发和维护的，支持Go语言的，类似Actor模型的框架。

* 基于Protobuf实现类似RPC的定义风格支持。

* 目前为止，Atomos只支持Go语言。当然，因为Cosmos节点之间的通讯方式是Protobuf，因此想要支持其它语言并不难。

为什么会有Atomos这个想法？

* 目前Go主流的实现并发的方式，通常都是CSP或者Mutex锁，但这些并发模型都有其本身的使用难度，开发人员和项目都可能需要血泪的教训，才会明白这些点。 因此，让开发人员从这些并发问题解放出来，无论对开发人员的身心健康，还是对项目的稳健发展，都是有利的。

* Actor模型是行业内比较常见的并发模型，首先，很多开发人员已经习惯了这种Actor模型提供的并发代码风格。其次，Actor模型的而且确把开发人员从并发问题中解放出来了，开发人员只需要通过消息的收发，并能轻松实现并发逻辑。

* Go目前还没有特别成熟的Actor模型框架，实际上Actor模型本身也有缺陷，例如完美遵循Actor模型的框架，在发送时，它们的消息是完全异步的，无法编写同步逻辑，只能通过回调实现；在处理消息时，还需要开发者自己实现一个消息的处理循环，这些对于Go来说都是不方便的。

* 目前主流的Actor模型框架都是弱类型的，但如果我们想实现强类型，想通过强类型来换取确定性，就很难。

如何实现一个解决以上问题的框架？

* 定位——支持Go语言、改良的Actor并发模型。 

* 使用Protobuf进行模型定义，更加方便易用的Modeling方式。

本框架哲学层面上的定义

1. Atomos——原子，是正在运行中的实例。它是本框架中的最小运行单位，代表着在本框架中不可以在拆分的单个算力，对应着一个独立运行的goroutine。是和Actor模型中Actor所对等的单位。

2. Element——元素，如果把Atomos看作面向对象中类的实例，那么Element则是类的概念。它负责Atomos的定义的同时，也是该类型Atomos的容器。
   
3. Cosmos Node——宇宙节点，每个单独运行的框架进程，都是一个独立的Cosmos Node，这些节点之间可以通过网络和Protobuf进行通讯。而每个Node，都是需要运行的Element的容器，同时你需要提供一个MainScript作为Node运行的引导。

4. Cosmos——宇宙，如果把这些所有的Cosmos Node都连起来，那么它们就成为了你想要的世界观的宇宙。

5. Wormhole——虫洞，连通Cosmos和外部世界的结点。实际上它是一个把网络连接服务抽象起来，并向Atomos提供读写操作功能的一种工具。

## 快速了解

让我们从项目的hello_world项目，快速了解如何使用本项目吧！

首先我们先了解proto的定义。

1. 在api目录的helloworld.proto文件中，定义Cosmos中会用到的Element。例如service Greeter就是一种Atomos的Element。为什么用service而不是element，是因为这个Protobuf的支持是通过扩展Protobuf实现的，所以还需要继续使用Protobuf中的关键词。

2. service Greeter中的rpc定义了Atomos实例所支持的消息调用，例如SayHello消息调用，生成后提供一个以HelloRequest作为参数，返回HelloReply的消息调用。

3. rpc Spawn是一个特殊的消息调用定义，它实际上是一个Atomos的启动消息，第一个参数HelloSpawnArg传入Greeter的启动参数，参数HelloData是Greeter的持久化对象。

4. 提前预热一下，因为Atomos实现了持久化支持，如果开发人员实现了ElementPersistence对象，那么在Atomos启动时，框架会向Spawn传入上次Halt的时候返回的Data对象。

接下来，我们尝试下protobuf文件生成Element的定义文件。

1. 在项目根目录执行./protoc-gen-go-atomos/build.sh

2. 该脚本会生成atomos的protobuf支持，并复制到go bin目录中，然后再调用protobuf生成命令，把helloworld.proto的相关定义生成到helloworld.atomos.pb.go文件中。

3. 在hello_world的element目录中，实现各种Element的定义，如在element_local.go文件中，LocalBoothElement结构体，实现了ElementDeveloper interface的定义。

* Load(MainId)，在Element加载的时候被调用，开发人员在这里处理Element的加载逻辑，如果有错误，应该返回错误，让Runnable停止运行。

* Unload()，在Element卸载的时候被调用。

* Info()，该Element的版本、debug level、初始atomos容器大小的信息。

* AtomConstructor()，返回该类型的Element类型的对象实例。

* 
