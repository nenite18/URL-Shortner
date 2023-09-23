package routes

import (
	"github.com/asaskevich/govalidator"
	"github.com/go-redis/redis/v8"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/google/uuid"
	"github.com/nenite18/URL-Shortner/api/database"
	"github.com/nenite18/URL-Shortner/api/helpers"
	"os"
	"strconv"
	"strings"
	"time"
)

type request struct {
	URL         string        `json:"url"`
	CustomShort string        `json:"short"`
	Expiry      time.Duration `json:"expiry"`
}

type response struct {
	URL             string        `json:"url"`
	CustomShort     string        `json:"short"`
	Expiry          time.Duration `json:"expiry"`
	XRateRemaining  int           `json:"rate_limit"`
	XRateLimitReset time.Duration `json:"rate_limit_reset"`
}

func ShortenURL(c *fiber.Ctx) error {
	log.Infof("context info passed by fiber context to shorten url handler: %v", c)
	body := new(request)
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"Error": "cannot parse json body",
		})
	}

	log.Info("Body of post request sent with URL:", body.URL)
	log.Info("Ip of address of ctx:", c.IP())

	r2 := database.CreateClient(1)
	defer r2.Close()
	log.Info("Starting redis DB server 2 for storing IP-ApiQuota")

	val, err := r2.Get(database.Ctx, c.IP()).Result()
	//Key IP_Address: API_Quota is not present in database
	if err == redis.Nil {

		log.Info("current IP does not exit in DB so creating new key")
		_ = r2.Set(database.Ctx, c.IP(), os.Getenv("API_QUOTA"), 30*time.Minute).Err()

	} else if err != nil {

		c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "can not establish connection with db",
		})

	} else {
		//Key[IP_ADDRESS] is already present
		//Checking FOR api_quota exceeded or not
		val, _ = r2.Get(database.Ctx, c.IP()).Result()
		valInt, _ := strconv.Atoi(val)
		if valInt <= 0 {
			limit, _ := r2.TTL(database.Ctx, c.IP()).Result()
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error":            "rate limit exceeded",
				"rate_limit_reset": limit / time.Nanosecond / time.Minute,
			})
		}

	}

	// check the provided URL is valid or not using go validator package
	if !govalidator.IsURL(body.URL) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"Error": "Invalid URL",
		})
	}

	//check if provided URL does not contain domain so to avoid any collisions
	if !helpers.RemoveDomainError(body.URL) {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"Error": "U cannot hack us",
		})
	}

	//enforce https, SSL in provided URL
	body.URL = helpers.EnforceHTTP(body.URL)

	r3 := database.CreateClient(2)
	log.Info("Starting redis DB server 3 for storing URl:CustomURL")
	defer r3.Close()

	// creating response body
	resp := response{
		URL:             body.URL,
		CustomShort:     "",
		Expiry:          0,
		XRateRemaining:  10,
		XRateLimitReset: 30,
	}

	// DECREMENTING  key value in DB 2 to implement API_QUOTA functionality
	log.Info("DECREMENTING  key value in DB 2 to implement API_QUOTA functionality")
	val, _ = r2.Get(database.Ctx, c.IP()).Result()
	resp.XRateRemaining, _ = strconv.Atoi(val)

	//calculating remaining time in refreshing the connection
	log.Info("calculating remaining time in refreshing the connection")
	ttl, _ := r2.TTL(database.Ctx, c.IP()).Result()
	resp.XRateLimitReset = ttl / time.Nanosecond / time.Minute

	//if expiry time not provided in requested body then to set it default 24 hrs
	log.Info("Expirty time not provided so setting it for deault expity of 24 hrs")
	if body.Expiry == 0 {
		resp.Expiry = 24 * time.Hour
	}

	var id string
	if val != "" {
		// checking if from same ip address same URL without a customURL is already present or not
		id, err = r3.Get(database.Ctx, body.URL).Result()
		if err == redis.Nil || body.CustomShort != "" {
			if body.CustomShort == "" {
				id = uuid.New().String()[:5]
			} else {
				id = body.CustomShort
			}
		} else {
			log.Info("Returning same customURL as IP address is same: ", id)
			//TODO: check from same ip if same url is being sent
			if c.IP() == strings.Split(id, ">")[0] {
				custId := strings.Split(id, ">")[1]
				resp.CustomShort = os.Getenv("DOMAIN") + "/" + custId
				log.Infof("Returning same customURL as IP address is same: %s Returned same ID: %s", c.IP(), custId)
				return c.Status(fiber.StatusOK).JSON(resp)
			}
		}
	}

	r := database.CreateClient(0)
	log.Infof("Starting redis DB server 1 for storing CustomURL:URL with id :%v", id)
	defer r.Close()

	log.Info("Setting key value in database 3 with key: %s, value: %s and expiry as: %v", body.URL, c.IP()+">"+id, resp.Expiry)
	err = r3.Set(database.Ctx, body.URL, c.IP()+">"+id, resp.Expiry).Err()

	val, _ = r.Get(database.Ctx, id).Result()
	if val != "" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error": "custom short is already in use",
		})
	}

	//creating key[id] for URL in database1
	err = r.Set(database.Ctx, id, body.URL, body.Expiry).Err()

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "unable to connect to server",
		})
	}

	//Decrementing the value for a key[IP address]
	r2.Decr(database.Ctx, c.IP())
	val, _ = r2.Get(database.Ctx, c.IP()).Result()
	resp.XRateRemaining, _ = strconv.Atoi(val)
	log.Info("decrement api quota for IP: ", c.IP())

	resp.CustomShort = os.Getenv("DOMAIN") + "/" + id

	return c.Status(fiber.StatusOK).JSON(resp)

}
