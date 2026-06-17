name: ivo
role: orders

Building the discount engine in the orders service. It calls the in-process
pricing package directly, no network hop. Once Mara's pricing service is stable
I plan to switch the engine over to call that service instead.
