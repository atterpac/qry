package main

// ptr returns a pointer to a string literal (for NULLable columns).
func ptr(s string) *string { return &s }

type user struct {
	Username    string
	Email       string
	DisplayName *string
	IsActive    bool
}

type product struct {
	Name        string
	Category    string
	Price       float64
	SKU         string
	Description *string
	InStock     bool
}

type tag struct {
	Name string
}

type order struct {
	UserID int
	Status string
	Total  float64
	Notes  *string
}

type orderItem struct {
	OrderID   int
	ProductID int
	Quantity  int
	UnitPrice float64
}

type productTag struct {
	ProductID int
	TagID     int
}

type event struct {
	EventType string
	UserID    int
	Payload   *string
}

// ── Seed Data ─────────────────────────────────────────────────────────────────

var seedUsers = []user{
	{"alice", "alice@example.com", ptr("Alice Johnson"), true},
	{"bob", "bob@example.com", ptr("Bob Smith"), true},
	{"charlie", "charlie@example.com", nil, true},
	{"diana", "diana@example.com", ptr("Diana Prince"), false},
	{"eve", "eve@example.com", ptr("Eve Torres"), true},
	{"frank", "frank@example.com", nil, true},
	{"grace", "grace@example.com", ptr("Grace Hopper"), true},
	{"hank", "hank@example.com", ptr("Hank Hill"), false},
	{"irene", "irene@example.com", nil, true},
	{"jack", "jack@example.com", ptr("Jack Reacher"), true},
	{"karen", "karen@example.com", ptr("Karen Page"), true},
	{"leo", "leo@example.com", nil, false},
	{"mona", "mona@example.com", ptr("Mona Lisa"), true},
	{"nate", "nate@example.com", ptr("Nate Silver"), true},
	{"olivia", "olivia@example.com", nil, true},
	{"paul", "paul@example.com", ptr("Paul Graham"), true},
	{"quinn", "quinn@example.com", nil, false},
	{"rosa", "rosa@example.com", ptr("Rosa Parks"), true},
	{"sam", "sam@example.com", ptr("Sam Altman"), true},
	{"tina", "tina@example.com", nil, true},
	{"ursula", "ursula@example.com", ptr("Ursula K. Le Guin"), true},
	{"victor", "victor@example.com", ptr("Victor Hugo"), false},
	{"wendy", "wendy@example.com", nil, true},
	{"xander", "xander@example.com", ptr("Xander Cage"), true},
	{"yara", "yara@example.com", ptr("Yara Shahidi"), true},
	{"zane", "zane@example.com", nil, true},
	{"amber", "amber@example.com", ptr("Amber Heard"), false},
	{"brian", "brian@example.com", ptr("Brian May"), true},
	{"clara", "clara@example.com", nil, true},
	{"derek", "derek@example.com", ptr("Derek Jeter"), true},
	{"elena", "elena@example.com", ptr("Elena Gilbert"), true},
	{"finn", "finn@example.com", nil, false},
	{"gina", "gina@example.com", ptr("Gina Rodriguez"), true},
	{"hugo", "hugo@example.com", ptr("Hugo Weaving"), true},
	{"isla", "isla@example.com", nil, true},
	{"jonas", "jonas@example.com", ptr("Jonas Salk"), true},
	{"kira", "kira@example.com", ptr("Kira Nerys"), false},
	{"liam", "liam@example.com", nil, true},
	{"maya", "maya@example.com", ptr("Maya Angelou"), true},
	{"noah", "noah@example.com", ptr("Noah Chomsky"), true},
	{"opal", "opal@example.com", nil, true},
	{"pete", "pete@example.com", ptr("Pete Sampras"), false},
	{"ruby", "ruby@example.com", ptr("Ruby Rose"), true},
	{"sean", "sean@example.com", nil, true},
	{"tara", "tara@example.com", ptr("Tara Reid"), true},
	{"uma", "uma@example.com", ptr("Uma Thurman"), true},
	{"vince", "vince@example.com", nil, false},
	{"willa", "willa@example.com", ptr("Willa Cather"), true},
	{"xena", "xena@example.com", ptr("Xena Warrior"), true},
	{"yusuf", "yusuf@example.com", nil, true},
}

var seedProducts = []product{
	{"Mechanical Keyboard", "electronics", 149.99, "ELEC-001", ptr("Cherry MX Brown switches, RGB backlight"), true},
	{"Wireless Mouse", "electronics", 59.99, "ELEC-002", nil, true},
	{"USB-C Hub", "electronics", 39.99, "ELEC-003", ptr("7-in-1 hub with HDMI and ethernet"), true},
	{"Monitor Stand", "accessories", 29.99, "ACCS-001", nil, true},
	{"Desk Lamp", "accessories", 44.99, "ACCS-002", ptr("LED adjustable brightness"), true},
	{"Laptop Sleeve", "accessories", 24.99, "ACCS-003", nil, false},
	{"Webcam HD", "electronics", 79.99, "ELEC-004", ptr("1080p with autofocus"), true},
	{"Noise Cancelling Headphones", "audio", 199.99, "AUD-001", ptr("Active noise cancellation, 30hr battery"), true},
	{"Bluetooth Speaker", "audio", 89.99, "AUD-002", nil, true},
	{"Condenser Microphone", "audio", 129.99, "AUD-003", ptr("USB condenser mic for streaming"), false},
	{"Standing Desk", "furniture", 499.99, "FURN-001", ptr("Electric height adjustable, 60x30 inches"), true},
	{"Ergonomic Chair", "furniture", 349.99, "FURN-002", nil, true},
	{"Cable Management Kit", "accessories", 19.99, "ACCS-004", ptr("Velcro ties and cable clips"), true},
	{"External SSD 1TB", "storage", 109.99, "STOR-001", nil, true},
	{"USB Flash Drive 64GB", "storage", 12.99, "STOR-002", ptr("USB 3.0, metal housing"), true},
	{"Mousepad XL", "accessories", 14.99, "ACCS-005", nil, true},
	{"Phone Mount", "accessories", 18.99, "ACCS-006", ptr("Adjustable clamp for desk"), false},
	{"HDMI Cable 6ft", "cables", 9.99, "CABL-001", nil, true},
	{"Ethernet Cable 10ft", "cables", 7.99, "CABL-002", ptr("Cat6, gold plated connectors"), true},
	{"USB-C Cable 3-pack", "cables", 14.99, "CABL-003", nil, true},
	{"Webcam Light Ring", "accessories", 24.99, "ACCS-007", ptr("10 inch ring light with tripod"), true},
	{"Power Strip", "electronics", 34.99, "ELEC-005", nil, true},
	{"Wrist Rest", "accessories", 16.99, "ACCS-008", ptr("Memory foam, ergonomic"), true},
	{"Screen Cleaner Kit", "accessories", 8.99, "ACCS-009", nil, false},
	{"Drawing Tablet", "electronics", 249.99, "ELEC-006", ptr("10x6 inch active area, 8192 pressure levels"), true},
	{"Portable Charger", "electronics", 29.99, "ELEC-007", nil, true},
	{"Smart Plug 2-pack", "electronics", 24.99, "ELEC-008", ptr("WiFi enabled, works with Alexa"), true},
	{"Desk Organizer", "accessories", 22.99, "ACCS-010", nil, true},
	{"Keyboard Wrist Pad", "accessories", 13.99, "ACCS-011", ptr("Gel filled, anti-slip base"), true},
	{"Air Duster Can", "accessories", 6.99, "ACCS-012", nil, true},
}

var seedTags = []tag{
	{"bestseller"},
	{"new-arrival"},
	{"on-sale"},
	{"premium"},
	{"eco-friendly"},
	{"limited-edition"},
	{"staff-pick"},
	{"bundle-eligible"},
	{"clearance"},
	{"trending"},
}

var seedOrders = []order{
	{1, "completed", 209.98, ptr("Please leave at front door")},
	{1, "completed", 59.99, nil},
	{2, "pending", 149.99, nil},
	{2, "completed", 539.98, ptr("Gift wrap requested")},
	{3, "shipped", 44.99, nil},
	{3, "completed", 89.99, ptr("Fragile items inside")},
	{4, "cancelled", 24.99, nil},
	{5, "completed", 199.99, nil},
	{5, "pending", 329.98, ptr("Deliver after 5pm")},
	{6, "completed", 79.99, nil},
	{7, "shipped", 129.99, nil},
	{7, "completed", 109.99, ptr("Second floor apartment")},
	{8, "pending", 499.99, nil},
	{9, "completed", 14.99, ptr("No rush on delivery")},
	{9, "completed", 34.99, nil},
	{10, "shipped", 249.99, nil},
	{10, "completed", 59.99, ptr("Call on arrival")},
	{11, "cancelled", 18.99, nil},
	{12, "completed", 349.99, nil},
	{12, "pending", 29.99, ptr("Leave with concierge")},
	{13, "completed", 89.99, nil},
	{14, "shipped", 24.99, nil},
	{14, "completed", 149.99, ptr("Needs signature")},
	{15, "pending", 79.99, nil},
	{16, "completed", 44.99, ptr("Back entrance please")},
	{16, "completed", 199.99, nil},
	{17, "shipped", 12.99, nil},
	{18, "completed", 39.99, ptr("Rush delivery")},
	{18, "pending", 109.99, nil},
	{19, "completed", 129.99, nil},
	{20, "shipped", 29.99, ptr("Ring doorbell twice")},
	{20, "completed", 499.99, nil},
	{21, "cancelled", 14.99, nil},
	{22, "completed", 349.99, ptr("Handle with care")},
	{22, "pending", 89.99, nil},
	{23, "completed", 59.99, nil},
	{24, "shipped", 24.99, nil},
	{24, "completed", 199.99, ptr("Weekend delivery preferred")},
	{25, "pending", 79.99, nil},
	{25, "completed", 44.99, nil},
	{26, "completed", 129.99, ptr("Include receipt")},
	{27, "shipped", 249.99, nil},
	{28, "completed", 34.99, nil},
	{28, "cancelled", 149.99, ptr("Changed mind")},
	{29, "completed", 109.99, nil},
	{30, "pending", 19.99, nil},
	{30, "completed", 89.99, ptr("Birthday gift")},
	{31, "shipped", 29.99, nil},
	{32, "completed", 199.99, nil},
	{33, "completed", 499.99, ptr("Office delivery")},
	{34, "pending", 59.99, nil},
	{35, "completed", 79.99, ptr("Leave at side gate")},
	{36, "shipped", 129.99, nil},
	{37, "completed", 24.99, nil},
	{38, "pending", 349.99, ptr("Assembly required - noted")},
	{39, "completed", 44.99, nil},
	{40, "completed", 14.99, nil},
	{40, "shipped", 249.99, ptr("Text when nearby")},
	{41, "cancelled", 89.99, nil},
	{42, "completed", 39.99, nil},
	{43, "pending", 199.99, ptr("No substitutions")},
	{44, "completed", 109.99, nil},
	{45, "shipped", 29.99, nil},
	{45, "completed", 149.99, ptr("Afternoon delivery")},
	{46, "pending", 79.99, nil},
	{47, "completed", 59.99, nil},
	{48, "completed", 34.99, ptr("Porch drop-off ok")},
	{48, "shipped", 499.99, nil},
	{49, "completed", 129.99, nil},
	{50, "pending", 89.99, ptr("Call before delivery")},
	{50, "completed", 24.99, nil},
	{1, "shipped", 349.99, nil},
	{2, "completed", 44.99, ptr("Express shipping")},
	{3, "pending", 199.99, nil},
	{5, "completed", 109.99, nil},
	{7, "shipped", 79.99, ptr("Garage delivery")},
	{10, "completed", 249.99, nil},
	{15, "pending", 59.99, nil},
	{20, "completed", 149.99, ptr("Final order of the month")},
	{25, "shipped", 89.99, nil},
}

var seedOrderItems = []orderItem{
	{1, 1, 1, 149.99},
	{1, 2, 1, 59.99},
	{2, 2, 1, 59.99},
	{3, 1, 1, 149.99},
	{4, 11, 1, 499.99},
	{4, 3, 1, 39.99},
	{5, 5, 1, 44.99},
	{6, 9, 1, 89.99},
	{7, 6, 1, 24.99},
	{8, 8, 1, 199.99},
	{9, 12, 1, 349.99},
	{10, 7, 1, 79.99},
	{11, 10, 1, 129.99},
	{12, 14, 1, 109.99},
	{13, 11, 1, 499.99},
	{14, 20, 1, 14.99},
	{15, 22, 1, 34.99},
	{16, 25, 1, 249.99},
	{17, 2, 1, 59.99},
	{18, 17, 1, 18.99},
	{19, 12, 1, 349.99},
	{20, 26, 1, 29.99},
	{21, 9, 1, 89.99},
	{22, 6, 1, 24.99},
	{23, 1, 1, 149.99},
	{24, 7, 1, 79.99},
	{25, 5, 1, 44.99},
	{26, 8, 1, 199.99},
	{27, 15, 1, 12.99},
	{28, 3, 1, 39.99},
	{29, 14, 1, 109.99},
	{30, 10, 1, 129.99},
	{31, 26, 1, 29.99},
	{32, 11, 1, 499.99},
	{33, 16, 1, 14.99},
	{34, 12, 1, 349.99},
	{35, 9, 1, 89.99},
	{36, 2, 1, 59.99},
	{37, 6, 1, 24.99},
	{38, 8, 1, 199.99},
	{39, 7, 1, 79.99},
	{40, 5, 1, 44.99},
	{41, 10, 1, 129.99},
	{42, 25, 1, 249.99},
	{43, 22, 1, 34.99},
	{44, 1, 1, 149.99},
	{45, 14, 1, 109.99},
	{46, 13, 1, 19.99},
	{47, 9, 1, 89.99},
	{48, 26, 1, 29.99},
	{49, 12, 1, 349.99},
	{50, 11, 1, 499.99},
	{51, 2, 1, 59.99},
	{52, 7, 1, 79.99},
	{53, 10, 1, 129.99},
	{54, 6, 1, 24.99},
	{55, 12, 1, 349.99},
	{56, 5, 1, 44.99},
	{57, 16, 1, 14.99},
	{58, 25, 1, 249.99},
	{59, 9, 1, 89.99},
	{60, 3, 1, 39.99},
	{61, 8, 1, 199.99},
	{62, 14, 1, 109.99},
	{63, 26, 1, 29.99},
	{64, 1, 1, 149.99},
	{65, 7, 1, 79.99},
	{66, 2, 1, 59.99},
	{67, 22, 1, 34.99},
	{68, 11, 1, 499.99},
	{69, 10, 1, 129.99},
	{70, 9, 1, 89.99},
	{71, 6, 1, 24.99},
	{72, 12, 1, 349.99},
	{73, 5, 1, 44.99},
	{74, 8, 1, 199.99},
	{75, 3, 1, 39.99},
	{76, 14, 1, 109.99},
	{77, 25, 1, 249.99},
	{78, 7, 1, 79.99},
	{79, 2, 1, 59.99},
	{80, 1, 1, 149.99},
	{9, 4, 1, 29.99},
	{20, 4, 1, 29.99},
	{32, 4, 1, 499.99},
	{4, 18, 1, 9.99},
	{50, 15, 1, 12.99},
	{68, 23, 1, 16.99},
	{38, 29, 1, 13.99},
	{55, 28, 1, 22.99},
	{10, 19, 1, 7.99},
	{25, 21, 1, 24.99},
	{40, 27, 1, 24.99},
	{60, 24, 1, 8.99},
	{70, 30, 1, 6.99},
	{15, 13, 1, 19.99},
}

var seedProductTags = []productTag{
	{1, 1},  // Mechanical Keyboard - bestseller
	{1, 7},  // Mechanical Keyboard - staff-pick
	{2, 1},  // Wireless Mouse - bestseller
	{2, 10}, // Wireless Mouse - trending
	{3, 2},  // USB-C Hub - new-arrival
	{4, 3},  // Monitor Stand - on-sale
	{5, 5},  // Desk Lamp - eco-friendly
	{6, 9},  // Laptop Sleeve - clearance
	{7, 2},  // Webcam HD - new-arrival
	{7, 10}, // Webcam HD - trending
	{8, 4},  // Noise Cancelling Headphones - premium
	{8, 1},  // Noise Cancelling Headphones - bestseller
	{9, 8},  // Bluetooth Speaker - bundle-eligible
	{10, 7}, // Condenser Microphone - staff-pick
	{11, 4}, // Standing Desk - premium
	{11, 5}, // Standing Desk - eco-friendly
	{12, 4}, // Ergonomic Chair - premium
	{13, 3}, // Cable Management Kit - on-sale
	{14, 1}, // External SSD - bestseller
	{14, 2}, // External SSD - new-arrival
	{15, 8}, // USB Flash Drive - bundle-eligible
	{16, 3}, // Mousepad XL - on-sale
	{17, 9}, // Phone Mount - clearance
	{18, 8}, // HDMI Cable - bundle-eligible
	{19, 8}, // Ethernet Cable - bundle-eligible
	{20, 8}, // USB-C Cable - bundle-eligible
	{21, 2}, // Webcam Light Ring - new-arrival
	{22, 10}, // Power Strip - trending
	{23, 5}, // Wrist Rest - eco-friendly
	{24, 9}, // Screen Cleaner Kit - clearance
	{25, 4}, // Drawing Tablet - premium
	{25, 6}, // Drawing Tablet - limited-edition
	{26, 10}, // Portable Charger - trending
	{27, 2}, // Smart Plug - new-arrival
	{28, 3}, // Desk Organizer - on-sale
	{29, 5}, // Keyboard Wrist Pad - eco-friendly
	{30, 3}, // Air Duster Can - on-sale
	{1, 10}, // Mechanical Keyboard - trending
	{8, 7},  // Noise Cancelling Headphones - staff-pick
	{11, 1}, // Standing Desk - bestseller
	{12, 7}, // Ergonomic Chair - staff-pick
	{14, 10}, // External SSD - trending
	{3, 8},  // USB-C Hub - bundle-eligible
	{5, 7},  // Desk Lamp - staff-pick
	{9, 10}, // Bluetooth Speaker - trending
	{22, 8}, // Power Strip - bundle-eligible
	{25, 7}, // Drawing Tablet - staff-pick
	{26, 8}, // Portable Charger - bundle-eligible
	{2, 8},  // Wireless Mouse - bundle-eligible
	{7, 1},  // Webcam HD - bestseller
	{12, 1}, // Ergonomic Chair - bestseller
	{16, 8}, // Mousepad XL - bundle-eligible
	{23, 7}, // Wrist Rest - staff-pick
	{27, 10}, // Smart Plug - trending
	{9, 3},  // Bluetooth Speaker - on-sale
	{13, 8}, // Cable Management Kit - bundle-eligible
	{21, 10}, // Webcam Light Ring - trending
	{28, 5}, // Desk Organizer - eco-friendly
	{29, 3}, // Keyboard Wrist Pad - on-sale
	{30, 8}, // Air Duster Can - bundle-eligible
}

var seedEvents = []event{
	{"page_view", 1, ptr(`{"page": "/products", "referrer": "google"}`)},
	{"page_view", 1, ptr(`{"page": "/products/1"}`)},
	{"add_to_cart", 1, ptr(`{"product_id": 1, "quantity": 1}`)},
	{"checkout", 1, ptr(`{"order_id": 1, "total": 209.98}`)},
	{"page_view", 2, nil},
	{"page_view", 2, ptr(`{"page": "/products"}`)},
	{"search", 2, ptr(`{"query": "keyboard", "results": 3}`)},
	{"add_to_cart", 2, ptr(`{"product_id": 1, "quantity": 1}`)},
	{"checkout", 2, ptr(`{"order_id": 3, "total": 149.99}`)},
	{"page_view", 3, ptr(`{"page": "/"}`)},
	{"page_view", 3, ptr(`{"page": "/products/5"}`)},
	{"add_to_cart", 3, ptr(`{"product_id": 5, "quantity": 1}`)},
	{"checkout", 3, ptr(`{"order_id": 5, "total": 44.99}`)},
	{"page_view", 4, nil},
	{"page_view", 5, ptr(`{"page": "/products/8"}`)},
	{"add_to_cart", 5, ptr(`{"product_id": 8, "quantity": 1}`)},
	{"checkout", 5, ptr(`{"order_id": 8, "total": 199.99}`)},
	{"page_view", 6, ptr(`{"page": "/"}`)},
	{"search", 6, ptr(`{"query": "webcam", "results": 2}`)},
	{"page_view", 7, ptr(`{"page": "/products/10"}`)},
	{"add_to_cart", 7, ptr(`{"product_id": 10, "quantity": 1}`)},
	{"checkout", 7, ptr(`{"order_id": 11, "total": 129.99}`)},
	{"page_view", 8, ptr(`{"page": "/products"}`)},
	{"search", 8, ptr(`{"query": "standing desk", "results": 1}`)},
	{"add_to_cart", 8, ptr(`{"product_id": 11, "quantity": 1}`)},
	{"page_view", 9, ptr(`{"page": "/"}`)},
	{"page_view", 9, nil},
	{"add_to_cart", 9, ptr(`{"product_id": 20, "quantity": 1}`)},
	{"checkout", 9, ptr(`{"order_id": 14, "total": 14.99}`)},
	{"page_view", 10, ptr(`{"page": "/products/25"}`)},
	{"add_to_cart", 10, ptr(`{"product_id": 25, "quantity": 1}`)},
	{"checkout", 10, ptr(`{"order_id": 16, "total": 249.99}`)},
	{"page_view", 11, nil},
	{"page_view", 12, ptr(`{"page": "/products/12"}`)},
	{"add_to_cart", 12, ptr(`{"product_id": 12, "quantity": 1}`)},
	{"checkout", 12, ptr(`{"order_id": 19, "total": 349.99}`)},
	{"page_view", 13, ptr(`{"page": "/products"}`)},
	{"search", 13, ptr(`{"query": "speaker bluetooth", "results": 1}`)},
	{"add_to_cart", 13, ptr(`{"product_id": 9, "quantity": 1}`)},
	{"checkout", 13, ptr(`{"order_id": 21, "total": 89.99}`)},
	{"page_view", 14, nil},
	{"page_view", 15, ptr(`{"page": "/products/7"}`)},
	{"add_to_cart", 15, ptr(`{"product_id": 7, "quantity": 1}`)},
	{"page_view", 16, ptr(`{"page": "/"}`)},
	{"page_view", 16, ptr(`{"page": "/products/5"}`)},
	{"add_to_cart", 16, ptr(`{"product_id": 5, "quantity": 1}`)},
	{"checkout", 16, ptr(`{"order_id": 25, "total": 44.99}`)},
	{"page_view", 17, nil},
	{"search", 17, ptr(`{"query": "usb cable", "results": 4}`)},
	{"page_view", 18, ptr(`{"page": "/products/3"}`)},
	{"add_to_cart", 18, ptr(`{"product_id": 3, "quantity": 1}`)},
	{"checkout", 18, ptr(`{"order_id": 28, "total": 39.99}`)},
	{"page_view", 19, ptr(`{"page": "/products/10"}`)},
	{"add_to_cart", 19, ptr(`{"product_id": 10, "quantity": 1}`)},
	{"checkout", 19, ptr(`{"order_id": 30, "total": 129.99}`)},
	{"page_view", 20, ptr(`{"page": "/"}`)},
	{"page_view", 20, nil},
	{"search", 20, ptr(`{"query": "portable charger", "results": 1}`)},
	{"add_to_cart", 20, ptr(`{"product_id": 26, "quantity": 1}`)},
	{"page_view", 21, ptr(`{"page": "/products"}`)},
	{"page_view", 22, ptr(`{"page": "/products/12"}`)},
	{"add_to_cart", 22, ptr(`{"product_id": 12, "quantity": 1}`)},
	{"checkout", 22, ptr(`{"order_id": 34, "total": 349.99}`)},
	{"page_view", 23, nil},
	{"page_view", 24, ptr(`{"page": "/products/6"}`)},
	{"add_to_cart", 24, ptr(`{"product_id": 6, "quantity": 1}`)},
	{"page_view", 25, ptr(`{"page": "/"}`)},
	{"search", 25, ptr(`{"query": "headphones noise", "results": 1}`)},
	{"add_to_cart", 25, ptr(`{"product_id": 8, "quantity": 1}`)},
	{"page_view", 26, ptr(`{"page": "/products/10"}`)},
	{"add_to_cart", 26, ptr(`{"product_id": 10, "quantity": 1}`)},
	{"checkout", 26, ptr(`{"order_id": 41, "total": 129.99}`)},
	{"page_view", 27, nil},
	{"page_view", 28, ptr(`{"page": "/products/25"}`)},
	{"add_to_cart", 28, ptr(`{"product_id": 25, "quantity": 1}`)},
	{"checkout", 28, ptr(`{"order_id": 42, "total": 249.99}`)},
	{"page_view", 29, ptr(`{"page": "/"}`)},
	{"search", 29, ptr(`{"query": "ssd external", "results": 1}`)},
	{"add_to_cart", 29, ptr(`{"product_id": 14, "quantity": 1}`)},
	{"checkout", 29, ptr(`{"order_id": 45, "total": 109.99}`)},
	{"page_view", 30, nil},
	{"page_view", 30, ptr(`{"page": "/products"}`)},
	{"search", 30, ptr(`{"query": "cable management", "results": 2}`)},
	{"add_to_cart", 30, ptr(`{"product_id": 13, "quantity": 1}`)},
	{"checkout", 30, ptr(`{"order_id": 46, "total": 19.99}`)},
	{"page_view", 31, ptr(`{"page": "/products/26"}`)},
	{"page_view", 32, nil},
	{"page_view", 33, ptr(`{"page": "/products/11"}`)},
	{"add_to_cart", 33, ptr(`{"product_id": 11, "quantity": 1}`)},
	{"checkout", 33, ptr(`{"order_id": 50, "total": 499.99}`)},
	{"page_view", 34, ptr(`{"page": "/"}`)},
	{"search", 34, ptr(`{"query": "mouse wireless", "results": 1}`)},
	{"page_view", 35, ptr(`{"page": "/products/7"}`)},
	{"add_to_cart", 35, ptr(`{"product_id": 7, "quantity": 1}`)},
	{"checkout", 35, ptr(`{"order_id": 52, "total": 79.99}`)},
	{"page_view", 36, nil},
	{"page_view", 37, ptr(`{"page": "/products/6"}`)},
	{"page_view", 38, ptr(`{"page": "/"}`)},
	{"search", 38, ptr(`{"query": "ergonomic chair", "results": 1}`)},
	{"add_to_cart", 38, ptr(`{"product_id": 12, "quantity": 1}`)},
	{"checkout", 38, ptr(`{"order_id": 55, "total": 349.99}`)},
}
