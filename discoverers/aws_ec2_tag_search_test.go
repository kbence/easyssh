package discoverers

func TestFailsWhenNoArgsGiven() {
	s := &awsEc2TagSearch{}
	args := []string{""}
	s.SetArgs(args.([]interface{}))
}
