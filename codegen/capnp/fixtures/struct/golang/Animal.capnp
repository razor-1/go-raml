
using Go = import "/go.capnp";
@0xf2b33c5cce93ae8a;

$Go.package("main");
$Go.import("main");
struct Animal {
  colours @0 :List(Text);
  name @1 :Text;
}
