name: ivo
role: orders

Building the new discount engine in the orders service. It calls the pricing
package in-process, no network hop — I'm relying on the in-process pricing
package staying exactly where it is through the end of next week while I wire the
engine up and test it.
